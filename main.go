package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DataDog/zstd"
)

type ModifiedFile struct { // This is for an individual decompressed file
	TypeSymbol       int64
	FileSymbol       int64
	ModifiedFilePath string
	PackageIndex     uint32
	DataOffset       uint32
	FileSize         uint32
}

type newFile struct { // Build manifest/package from this
	TypeSymbol       int64
	FileSymbol       int64
	ModifiedFilePath string
	FileSize         uint32
}

type fileGroup struct {
	currentData []byte
	fileIndex   uint32
	fileCount   int
}

const compressionLevel = zstd.BestSpeed

// TODO: debug rebuildPackageManifestCombo
// TODO: rewrite replace file
// TODO: multiple manifest version support

func main() {
	mode := flag.String("mode", "", "Either 'extract', 'build', or 'jsonmanifest'")
	packageName := flag.String("packageName", "", "File name of package, e.g. 48037dc70b0ecab2, 2b47aab238f60515, etc")
	dataDir := flag.String("dataDir", "", "Path of directory containing 'manifests' & 'packages' in ready-at-dawn-echo-arena/_data")
	inputDir := flag.String("inputDir", "", "Path of directory containing modified files (same structure as '-mode extract' output)")
	outputDir := flag.String("outputDir", "", "Path of directory to place modified package & manifest files")
	help := flag.Bool("help", false, "Print usage")
	flag.Parse()

	if *help || len(os.Args) == 1 || (*mode) == "" || (*outputDir) == "" {
		flag.Usage()
		return
	}

	if (*mode) != "extract" && (*mode) != "build" && (*mode) != "jsonmanifest" {
		fmt.Println("mode must be either 'extract', 'build', or 'jsonmanifest'")
		flag.Usage()
		return
	}

	if *(mode) == "build" && (*inputDir) == "" {
		fmt.Println("'build' must be used in conjunction with '-inputFolder'")
		flag.Usage()
		return
	}

	if (*mode) == "build" {
		files, err := scanPackageFiles((*inputDir))
		if err != nil {
			fmt.Printf("failed to scan %s", (*inputDir))
			panic(err)
		}

		if err := rebuildPackageManifestCombo(files, (*outputDir)); err != nil {
			fmt.Println(err)
			return
		}
		return
	}

	b, err := os.ReadFile((*dataDir) + "/manifests/" + (*packageName))
	if err != nil {
		fmt.Println("Failed to open manifest file, check dataDir path")
		return
	}

	compHeader := CompressedHeader{}
	decompBytes, err := decompressZSTD(b[binary.Size(compHeader):])
	if err != nil {
		fmt.Println("Failed to decompress manifest")
		fmt.Println(hex.Dump(b[binary.Size(compHeader):][:256]))
		fmt.Println(err)
		return
	}

	buf := bytes.NewReader(b)
	err = binary.Read(buf, binary.LittleEndian, &compHeader)
	if err != nil {
		fmt.Println("failed to marshal manifest into struct")
		return
	}

	if len(b[binary.Size(compHeader):]) != int(compHeader.CompressedSize) || len(decompBytes) != int(compHeader.UncompressedSize) {
		fmt.Println("Manifest header does not match actual size of manifest")
		return
	}

	manifest := unmarshalManifest(decompBytes)

	if (*mode) == "extract" {
		if err := extractFilesFromPackage(manifest, (*dataDir), (*packageName), (*outputDir)); err != nil {
			fmt.Println("Error extracting files: ", err)
		}
		return
	} else if (*mode) == "jsonmanifest" {
		jFile, err := os.OpenFile("manifestdebug.json", os.O_RDWR|os.O_CREATE, 0777)
		if err != nil {
			return
		}
		jBytes, _ := json.MarshalIndent(manifest, "", "	")
		jFile.Write(jBytes)
		jFile.Close()
	}
}

func rebuildPackageManifestCombo(fileMap [][]newFile, outputFolder string) error {
	totalFileCount := 0
	for _, v := range fileMap {
		totalFileCount += len(v)
	}
	fmt.Printf("Building from %d files\n", totalFileCount)
	manifest := FullManifest{
		Header: Manifest_header{
			PackageCount:  1,
			Unk1:          0,
			Unk2:          0,
			FrameContents: Header_chunk{SectionSize: 0, Unk1: 0, Unk2: 0, ElementSize: 32, Count: 0, ElementCount: 0},
			SomeStructure: Header_chunk{SectionSize: 0, Unk1: 0, Unk2: 0, ElementSize: 40, Count: 0, ElementCount: 0},
			Frames:        Header_chunk{SectionSize: 0, Unk1: 0, Unk2: 0, ElementSize: 16, Count: 0, ElementCount: 0},
		},
		FrameContents: make([]Frame_contents, totalFileCount),
		SomeStructure: make([]Some_structure1, totalFileCount),
		FileInfo:      []Frame{},
	}

	currentPackageIndex := uint32(0)
	currentFileGroup := fileGroup{}
	totalFilesWritten := 0

	// preserving chunk grouping, temporary until I can figure out what's causing echo to break
	for _, files := range fileMap {
		if !bytes.Equal(currentFileGroup.currentData, []byte{}) {
			fmt.Println("Writing chunk to package")
			if err := writeChunkToPackage(&manifest, currentFileGroup, &currentPackageIndex, outputFolder); err != nil {
				return err
			}
			currentFileGroup.currentData = nil
			currentFileGroup.fileIndex++
			currentFileGroup.fileCount = 0
		}
		fmt.Printf("on file %d/%d\n", totalFilesWritten, totalFileCount)
		for _, file := range files {
			toWrite, err := os.ReadFile(file.ModifiedFilePath)
			if err != nil {
				return err
			}

			frameContentsEntry := Frame_contents{
				T:             file.TypeSymbol,
				FileSymbol:    file.FileSymbol,
				FileIndex:     currentFileGroup.fileIndex,
				DataOffset:    uint32(len(currentFileGroup.currentData)),
				Size:          uint32(len(toWrite)),
				SomeAlignment: 1,
			}
			someStructureEntry := Some_structure1{
				T:          file.TypeSymbol,
				FileSymbol: file.FileSymbol,
				Unk1:       0,
				Unk2:       0,
				AssetType:  0,
			}

			manifest.FrameContents[totalFilesWritten] = frameContentsEntry
			manifest.SomeStructure[totalFilesWritten] = someStructureEntry
			manifest.Header.FrameContents = incrementHeaderChunk(manifest.Header.FrameContents, 1)
			manifest.Header.SomeStructure = incrementHeaderChunk(manifest.Header.SomeStructure, 1)

			// 0byte entry doesn't need to be written
			if len(toWrite) == 0 && currentFileGroup.fileCount != 0 {
				currentFileGroup.fileCount++
				continue
			}

			totalFilesWritten++
			currentFileGroup.fileCount++
			currentFileGroup.currentData = append(currentFileGroup.currentData, toWrite...)
		}
	}

	// write weird data

	for i := uint32(0); i < manifest.Header.PackageCount; i++ {
		packageStats, err := os.Stat(fmt.Sprintf("%s/packages/package_%d", outputFolder, i))
		if err != nil {
			fmt.Println("failed to stat package for weirddata writing")
			return err
		}
		if i == 0 {
			manifest.FileInfo[len(manifest.FileInfo)-1].NextEntryOffset = uint32(packageStats.Size())
			manifest.FileInfo[len(manifest.FileInfo)-1].NextEntryPackageIndex = 0
			continue
		}
		newEntry := Frame{
			CompressedSize:        0, // TODO: find out what this actually is
			DecompressedSize:      0,
			NextEntryPackageIndex: i,
			NextEntryOffset:       uint32(packageStats.Size()),
		}
		manifest.FileInfo = append(manifest.FileInfo, newEntry)
		manifest.Header.Frames = incrementHeaderChunk(manifest.Header.Frames, 1)
	}

	newEntry := Frame{
		CompressedSize:        0, // TODO: find out what this actually is
		DecompressedSize:      0,
		NextEntryPackageIndex: 0,
		NextEntryOffset:       0,
	}

	manifest.FileInfo = append(manifest.FileInfo, newEntry)
	manifest.Header.Frames = incrementHeaderChunk(manifest.Header.Frames, 1)

	// write out manifest
	fmt.Println("Writing manifest")
	if err := writeManifest(manifest, outputFolder, "package"); err != nil {
		return err
	}
	return nil
}

// Takes a fileGroup and writes the data contained in it to a package
// TODO: calculate activePackageNum dynamically instead of this insanity
func writeChunkToPackage(manifest *FullManifest, currentFileGroup fileGroup, activePackageNum *uint32, outputFolder string) error {
	packageSizeLimit := 2147483647 // uint32 limit
	compFile, err := zstd.CompressLevel(nil, currentFileGroup.currentData, compressionLevel)
	if err != nil {
		return err
	}
	currentPackagePath := fmt.Sprintf("%s/packages/package_%d", outputFolder, *activePackageNum)
	currentPackageSize := int64(0)
	fInfo, err := os.Stat(currentPackagePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	} else if err == nil {
		currentPackageSize = fInfo.Size()
	}

	if int64(len(compFile))+currentPackageSize > int64(packageSizeLimit) {
		*activePackageNum++
		manifest.Header.PackageCount = (*activePackageNum) + 1
		manifest.FileInfo[len(manifest.FileInfo)-1].NextEntryPackageIndex = (*activePackageNum)
		manifest.FileInfo[len(manifest.FileInfo)-1].NextEntryOffset = 0
		currentPackagePath = fmt.Sprintf("%s/packages/package_%d", outputFolder, (*activePackageNum))
	}
	os.MkdirAll(outputFolder+"/packages", 0777)
	f, err := os.OpenFile(currentPackagePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0777)
	if err != nil {
		return err
	}
	defer f.Close()
	numWrittenBytes, err := f.Write(compFile)
	if err != nil {
		return err
	}
	if numWrittenBytes != len(compFile) {
		return errors.New("failed to write all bytes to package")
	}
	debugFile, _ := os.OpenFile(fmt.Sprintf("./debug/%d.zst", currentFileGroup.fileIndex), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0777)
	debugFile.Write(compFile)
	debugFile.Close()
	fmt.Printf("wrote file %d, %d compressed (%d uncompressed) bytes to %s\n", currentFileGroup.fileIndex, numWrittenBytes, len(currentFileGroup.currentData), currentPackagePath)

	nextOffset := uint32(0)
	if len(manifest.FileInfo) > 0 {
		nextOffset = manifest.FileInfo[len(manifest.FileInfo)-1].NextEntryOffset
	}

	newEntry := Frame{
		CompressedSize:        uint32(numWrittenBytes),
		DecompressedSize:      uint32(len(currentFileGroup.currentData)),
		NextEntryPackageIndex: (*activePackageNum),
		NextEntryOffset:       nextOffset + uint32(numWrittenBytes),
	}

	manifest.FileInfo = append(manifest.FileInfo, newEntry)
	manifest.Header.Frames.SectionSize += uint64(binary.Size(Frame{}))
	manifest.Header.Frames.Count++
	manifest.Header.Frames.ElementCount++

	return nil
}

func scanPackageFiles(inputFolder string) ([][]newFile, error) {
	// there has to be a better way to do this
	filestats, _ := os.ReadDir(inputFolder)
	files := make([][]newFile, len(filestats))
	filepath.Walk(inputFolder, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Println(err)
			return err
		}
		if info.IsDir() {
			return nil
		}
		newFile := newFile{}
		newFile.ModifiedFilePath = path
		newFile.FileSize = uint32(info.Size())
		dir1 := strings.Split(path, "\\")[len(strings.Split(path, "\\"))-3]
		dir2 := strings.Split(path, "\\")[len(strings.Split(path, "\\"))-2]
		dir3 := strings.Split(path, "\\")[len(strings.Split(path, "\\"))-1]
		chunkNum, _ := strconv.ParseInt(dir1, 10, 64)
		newFile.TypeSymbol, _ = strconv.ParseInt(dir2, 10, 64)
		newFile.FileSymbol, _ = strconv.ParseInt(dir3, 10, 64)
		files[chunkNum] = append(files[chunkNum], newFile)
		return nil
	})
	return files, nil
}

func extractFilesFromPackage(fullManifest FullManifest, dataDir string, packageName string, outputFolder string) error {
	files, err := splitPackages(fullManifest, dataDir, packageName)
	if err != nil {
		fmt.Println("failed to split package, check input")
		return err
	}
	totalFilesWritten := 0
	for k, v := range files {
		if bytes.Equal(v, []byte{}) {
			fmt.Printf("skipping extraction of empty file %d\n", k)
			continue
		}

		fmt.Printf("Decompressing and extracting files contained in file index %d, %d/%d\n", k, totalFilesWritten, fullManifest.Header.FrameContents.Count)
		decompBytes, err := decompressZSTD(v)
		if err != nil {
			return err
		}

		if len(decompBytes) != int(fullManifest.FileInfo[k].DecompressedSize) {
			return fmt.Errorf("size of decompressed data does not match manifest for file %d, is %d but should be %d", k, len(decompBytes), fullManifest.FileInfo[k].DecompressedSize)
		}

		for _, v2 := range fullManifest.FrameContents {
			if v2.FileIndex != k {
				continue
			}
			fileName := strconv.FormatInt(v2.FileSymbol, 10)
			fileType := strconv.FormatInt(v2.T, 10)
			os.MkdirAll(fmt.Sprintf("%s/%d/%s", outputFolder, v2.FileIndex, fileType), 0777)

			file, err := os.OpenFile(fmt.Sprintf("%s/%d/%s/%s", outputFolder, v2.FileIndex, fileType, fileName), os.O_RDWR|os.O_CREATE, 0777)
			if err != nil {
				fmt.Println(err)
				continue
			}

			file.Write(decompBytes[v2.DataOffset : v2.DataOffset+v2.Size])
			file.Close()
			totalFilesWritten++
		}
	}
	return nil
}

func splitPackages(manifest FullManifest, dataDir string, packageName string) (map[uint32][]byte, error) { // this should Just Work and have no issues
	splitFiles := make(map[uint32][]byte)
	packages := make(map[uint32]*os.File)

	for i := 0; i < int(manifest.Header.PackageCount); i++ {
		pFilePath := fmt.Sprintf("%s/packages/%s_%d", dataDir, packageName, i)
		f, err := os.Open(pFilePath)
		if err != nil {
			fmt.Printf("failed to open package %s\n", pFilePath)
			return nil, err
		}
		packages[uint32(i)] = f
		defer f.Close()
	}
	activeFile := packages[0]
	for k, v := range manifest.FileInfo {
		fmt.Printf("reading file %d\n", k)
		splitFile := make([]byte, v.CompressedSize)
		_, err := io.ReadAtLeast(activeFile, splitFile, int(v.CompressedSize))

		if err != nil && v.DecompressedSize == 0 {
			fmt.Println("skipping invalid entry with 0 decompressed size")
			splitFile = []byte{}
		} else if err != nil {
			fmt.Println("failed to read file, check input")
			return nil, err
		}

		splitFiles[uint32(k)] = splitFile

		activeFile = packages[v.NextEntryPackageIndex]
		activeFile.Seek(int64(v.NextEntryOffset), 0)
	}

	return splitFiles, nil
}

func incrementHeaderChunk(chunk Header_chunk, amount int) Header_chunk {
	for i := 0; i < amount; i++ {
		chunk.Count++
		chunk.ElementCount++
		chunk.SectionSize += uint64(chunk.ElementSize)
	}
	return chunk
}

func decrementHeaderChunk(chunk Header_chunk, amount int) Header_chunk {
	for i := 0; i < amount; i++ {
		chunk.Count--
		chunk.ElementCount--
		chunk.SectionSize -= uint64(binary.Size(Frame{}))
	}
	return chunk
}

func unmarshalManifest(b []byte) FullManifest {
	// copied straight from me three weeks ago, i'm not making this look nicer
	currentOffset := 200 // 192 for evr - 200 for le2
	manifestHeader := Manifest_header{}
	buf := bytes.NewReader(b[:currentOffset]) // ideally this shouldn't be hardcoded but for now it's fine
	err := binary.Read(buf, binary.LittleEndian, &manifestHeader)
	if err != nil {
		panic(err)
	}

	frameContents := make([]Frame_contents, manifestHeader.FrameContents.ElementCount)
	frameContentsLength := binary.Size(frameContents)
	frameContentsBuf := bytes.NewReader(b[currentOffset : currentOffset+frameContentsLength])
	currentOffset = currentOffset + frameContentsLength
	if err := binary.Read(frameContentsBuf, binary.LittleEndian, &frameContents); err != nil {
		fmt.Println("Failed to marshal frameContents into struct")
		panic(err)
	}

	someStructure1 := make([]Some_structure1, manifestHeader.SomeStructure.ElementCount)
	someStructure1Length := binary.Size(someStructure1)
	someStructure1Buf := bytes.NewReader(b[currentOffset : currentOffset+someStructure1Length])
	currentOffset = currentOffset + someStructure1Length
	if err := binary.Read(someStructure1Buf, binary.LittleEndian, &someStructure1); err != nil {
		fmt.Println("Failed to marshal someStructure1 into struct")
		panic(err)
	}

	currentOffset += 8 // skip padding(?)

	manifestFileInfo := make([]Frame, manifestHeader.Frames.ElementCount)
	b = append(b, make([]byte, 8)...) // hacky way to read last bit of manifest as frame
	manifestFileInfoBuf := bytes.NewReader(b[currentOffset:])
	if err := binary.Read(manifestFileInfoBuf, binary.LittleEndian, &manifestFileInfo); err != nil {
		fmt.Println("Failed to marshal manifestFileInfo into struct")
		panic(err)
	}

	fullManifest := FullManifest{
		manifestHeader,
		frameContents,
		someStructure1,
		[8]byte{},
		manifestFileInfo,
	}

	return fullManifest
}

func decompressZSTD(b []byte) ([]byte, error) {
	decomp, err := zstd.Decompress(nil, b)
	if err != nil {
		return nil, err
	}
	return decomp, nil
}

func writeManifest(manifest FullManifest, outputFolder string, packageName string) error {
	wbuf := bytes.NewBuffer(nil)

	var data = []any{
		manifest.Header,
		manifest.FrameContents,
		manifest.SomeStructure,
		[8]byte{},
		manifest.FileInfo,
	}
	for _, v := range data {
		err := binary.Write(wbuf, binary.LittleEndian, v)
		if err != nil {
			fmt.Println("binary.Write failed:", err)
		}
	}

	jFile, err := os.OpenFile("manifest.json", os.O_RDWR|os.O_CREATE, 0777)
	if err != nil {
		return err
	}
	jBytes, _ := json.Marshal(manifest)
	jFile.Write(jBytes)
	jFile.Close()

	os.MkdirAll(outputFolder+"/manifests/", 0777)
	file, err := os.OpenFile(outputFolder+"/manifests/"+packageName, os.O_RDWR|os.O_CREATE, 0777)
	if err != nil {
		return err
	}
	manifestBytes := wbuf.Bytes()
	file.Write(compressManifest(manifestBytes[:len(manifestBytes)-8])) // bit of a hacky way to get half of the fileInfo entry written, but it's w/e
	file.Close()
	return nil
}

func compressManifest(b []byte) []byte {
	zstdBytes, err := zstd.CompressLevel(nil, b, compressionLevel)
	if err != nil {
		fmt.Println("error compressing manifest")
		panic(err)
	}
	cHeader := CompressedHeader{
		[4]byte{0x5A, 0x53, 0x54, 0x44}, // Z S T D
		uint32(binary.Size(CompressedHeader{})),
		uint64(len(b)),
		uint64(len(zstdBytes)),
	}

	fBuf := bytes.NewBuffer(nil)
	binary.Write(fBuf, binary.LittleEndian, cHeader)

	return append(fBuf.Bytes(), zstdBytes...)
}
