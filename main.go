package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/DataDog/zstd"
	evrm "github.com/goopsie/echoModifyFiles/evrManifests"
)

type CompressedHeader struct { // seems to be the same across every manifest
	Magic            [4]byte
	HeaderSize       uint32
	UncompressedSize uint64
	CompressedSize   uint64
}

type newFile struct { // Build manifest/package from this
	TypeSymbol       int64
	FileSymbol       int64
	ModifiedFilePath string
	FileSize         uint32
}

type fileGroup struct {
	currentData bytes.Buffer
	fileIndex   uint32
	fileCount   int
}

const compressionLevel = zstd.BestSpeed

var (
	mode         string
	manifestType string
	packageName  string
	dataDir      string
	inputDir     string
	outputDir    string
	help         bool
)

// TODO: debug rebuildPackageManifestCombo
// TODO: rewrite replace file

func init() {
	flag.StringVar(&mode, "mode", "", "must be one of the following: 'extract', 'build', 'replace', 'jsonmanifest'")
	flag.StringVar(&manifestType, "manifestType", "5932408047-EVR", "See readme for updated list of manifest types.")
	flag.StringVar(&packageName, "packageName", "package", "File name of package, e.g. 48037dc70b0ecab2, 2b47aab238f60515, etc.")
	flag.StringVar(&dataDir, "dataDir", "", "Path of directory containing 'manifests' & 'packages' in ready-at-dawn-echo-arena/_data")
	flag.StringVar(&inputDir, "inputDir", "", "Path of directory containing modified files (same structure as '-mode extract' output)")
	flag.StringVar(&outputDir, "outputDir", "", "Path of directory to place modified package & manifest files")
	flag.BoolVar(&help, "help", false, "Print usage")
	flag.Parse()

	if help {
		flag.Usage()
		os.Exit(0)
	}

	if mode == "jsonmanifest" && dataDir == "" {
		fmt.Println("'-mode jsonmanifest' must be used in conjunction with '-dataDir'")
		os.Exit(1)
	}

	if help || len(os.Args) == 1 || mode == "" || outputDir == "" {
		flag.Usage()
		os.Exit(1)
	}

	if mode != "extract" && mode != "build" && mode != "replace" && mode != "jsonmanifest" {
		fmt.Println("mode must be one of the following: 'extract', 'build', 'replace', 'jsonmanifest'")
		flag.Usage()
		os.Exit(1)
	}

	if mode == "build" && inputDir == "" {
		fmt.Println("'-mode build' must be used in conjunction with '-inputDir'")
		flag.Usage()
		os.Exit(1)
	}
}

func main() {
	if mode == "build" {
		files, err := scanPackageFiles()
		if err != nil {
			fmt.Printf("failed to scan %s", inputDir)
			panic(err)
		}

		if err := rebuildPackageManifestCombo(files); err != nil {
			fmt.Println(err)
			return
		}
		return
	}

	b, err := os.ReadFile(dataDir + "/manifests/" + packageName)
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

	manifest, err := evrm.MarshalManifest(decompBytes, manifestType)
	if err != nil {
		fmt.Println("Error creating manifest: ", err)
		panic(err)
	}

	if mode == "extract" {
		if err := extractFilesFromPackage(manifest); err != nil {
			fmt.Println("Error extracting files: ", err)
		}
		return
	} else if mode == "replace" {
		files, err := scanPackageFiles()
		if err != nil {
			fmt.Printf("failed to scan %s", inputDir)
			panic(err)
		}

		if err := replaceFiles(files, manifest); err != nil {
			fmt.Println(err)
			return
		}

	} else if mode == "jsonmanifest" {
		jFile, err := os.OpenFile("manifestdebug.json", os.O_RDWR|os.O_CREATE, 0777)
		if err != nil {
			return
		}
		jBytes, _ := json.MarshalIndent(manifest, "", "	")
		jFile.Write(jBytes)
		jFile.Close()
	}
}

func replaceFiles(fileMap [][]newFile, manifest evrm.EvrManifest) error {
	// really need to find out grouping rules... [][]newFile is really ugly

	return nil
}

func decompressZSTD(b []byte) ([]byte, error) {
	decomp, err := zstd.Decompress(nil, b)
	if err != nil {
		return nil, err
	}
	return decomp, nil
}

func rebuildPackageManifestCombo(fileMap [][]newFile) error {
	totalFileCount := 0
	for _, v := range fileMap {
		totalFileCount += len(v)
	}
	fmt.Printf("Building from %d files\n", totalFileCount)
	manifest := evrm.EvrManifest{
		Header: evrm.ManifestHeader{
			PackageCount:  1,
			Unk1:          0,
			Unk2:          0,
			FrameContents: evrm.HeaderChunk{SectionSize: 0, Unk1: 0, Unk2: 0, ElementSize: 32, Count: 0, ElementCount: 0},
			SomeStructure: evrm.HeaderChunk{SectionSize: 0, Unk1: 0, Unk2: 0, ElementSize: 40, Count: 0, ElementCount: 0},
			Frames:        evrm.HeaderChunk{SectionSize: 0, Unk1: 0, Unk2: 0, ElementSize: 16, Count: 0, ElementCount: 0},
		},
		FrameContents: make([]evrm.FrameContents, totalFileCount),
		SomeStructure: make([]evrm.SomeStructure, totalFileCount),
		Frames:        []evrm.Frame{},
	}

	currentFileGroup := fileGroup{}
	totalFilesWritten := 0

	// preserving chunk grouping, temporary until I can figure out grouping rules
	for _, files := range fileMap {
		if currentFileGroup.currentData.Len() != 0 {
			if err := appendChunkToPackages(&manifest, currentFileGroup); err != nil {
				return err
			}
			currentFileGroup.currentData.Reset()
			currentFileGroup.fileIndex++
			currentFileGroup.fileCount = 0
		}
		for _, file := range files {
			toWrite, err := os.ReadFile(file.ModifiedFilePath)
			if err != nil {
				return err
			}

			frameContentsEntry := evrm.FrameContents{
				T:             file.TypeSymbol,
				FileSymbol:    file.FileSymbol,
				FileIndex:     currentFileGroup.fileIndex,
				DataOffset:    uint32(currentFileGroup.currentData.Len()),
				Size:          uint32(len(toWrite)),
				SomeAlignment: 1,
			}
			someStructureEntry := evrm.SomeStructure{
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

			totalFilesWritten++
			currentFileGroup.fileCount++
			currentFileGroup.currentData.Write(toWrite)
		}
	}
	if currentFileGroup.currentData.Len() > 0 {
		if err := appendChunkToPackages(&manifest, currentFileGroup); err != nil {
			return err
		}
		currentFileGroup.currentData.Reset()
		currentFileGroup.fileIndex++
		currentFileGroup.fileCount = 0
	}
	fmt.Printf("finished writing package data, %d files in %d packages\n", totalFilesWritten, manifest.Header.PackageCount)

	// write weird data
	// not necessary from what i can tell but just in case

	for i := uint32(0); i < manifest.Header.PackageCount; i++ {
		packageStats, err := os.Stat(fmt.Sprintf("%s/packages/%s_%d", outputDir, packageName, i))
		if err != nil {
			fmt.Println("failed to stat package for weirddata writing")
			return err
		}
		newEntry := evrm.Frame{
			CurrentPackageIndex: i,
			CurrentOffset:       uint32(packageStats.Size()),
			CompressedSize:      0, // TODO: find out what this actually is
			DecompressedSize:    0,
		}
		manifest.Frames = append(manifest.Frames, newEntry)
		manifest.Header.Frames = incrementHeaderChunk(manifest.Header.Frames, 1)
	}

	newEntry := evrm.Frame{} // CompressedSize here is a populated field, but i don't know what it's used for

	manifest.Frames = append(manifest.Frames, newEntry)
	manifest.Header.Frames = incrementHeaderChunk(manifest.Header.Frames, 1)

	// write out manifest
	fmt.Println("Writing manifest")
	if err := writeManifest(manifest); err != nil {
		return err
	}
	return nil
}

// Takes a fileGroup, appends the data contained into whichever package set is specified.
// Modifies provided manifest to match the appended data.
func appendChunkToPackages(manifest *evrm.EvrManifest, currentFileGroup fileGroup) error {
	os.MkdirAll(fmt.Sprintf("%s/packages", outputDir), 0777)

	// current frame var to avoid confusing myself
	cEntry := evrm.Frame{}
	activePackageNum := uint32(0)
	if len(manifest.Frames) > 0 {
		cEntry = manifest.Frames[len(manifest.Frames)-1]
		activePackageNum = cEntry.CurrentPackageIndex
	}
	compFile, err := zstd.CompressLevel(nil, currentFileGroup.currentData.Bytes(), compressionLevel)
	if err != nil {
		return err
	}

	currentPackagePath := fmt.Sprintf("%s/packages/%s_%d", outputDir, packageName, activePackageNum)

	if int(cEntry.CurrentOffset+cEntry.CompressedSize)+len(compFile) > math.MaxInt32 {
		activePackageNum++
		manifest.Header.PackageCount = activePackageNum + 1
		currentPackagePath = fmt.Sprintf("%s/packages/%s_%d", outputDir, packageName, activePackageNum)
	}

	f, err := os.OpenFile(currentPackagePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0777)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(compFile)
	if err != nil {
		return err
	}

	newEntry := evrm.Frame{
		CurrentPackageIndex: activePackageNum,
		CurrentOffset:       cEntry.CurrentOffset + cEntry.CompressedSize,
		CompressedSize:      uint32(len(compFile)),
		DecompressedSize:    uint32(currentFileGroup.currentData.Len()),
	}
	if newEntry.CurrentOffset+newEntry.CompressedSize > math.MaxInt32 {
		newEntry.CurrentOffset = 0
	}

	manifest.Frames = append(manifest.Frames, newEntry)
	manifest.Header.Frames = incrementHeaderChunk(manifest.Header.Frames, 1)

	return nil
}

func scanPackageFiles() ([][]newFile, error) {
	// there has to be a better way to do this
	filestats, _ := os.ReadDir(inputDir)
	files := make([][]newFile, len(filestats))
	err := filepath.Walk(inputDir, func(path string, info os.FileInfo, err error) error {
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
		chunkNum, err := strconv.ParseInt(dir1, 10, 64)
		if err != nil {
			return err
		}
		newFile.TypeSymbol, err = strconv.ParseInt(dir2, 10, 64)
		if err != nil {
			return err
		}
		newFile.FileSymbol, err = strconv.ParseInt(dir3, 10, 64)
		if err != nil {
			return err
		}
		files[chunkNum] = append(files[chunkNum], newFile)
		return nil
	})

	if err != nil {
		return nil, err
	}
	return files, nil
}

func extractFilesFromPackage(fullManifest evrm.EvrManifest) error {
	packages := make(map[uint32]*os.File)
	totalFilesWritten := 0

	for i := 0; i < int(fullManifest.Header.PackageCount); i++ {
		pFilePath := fmt.Sprintf("%s/packages/%s_%d", dataDir, packageName, i)
		f, err := os.Open(pFilePath)
		if err != nil {
			fmt.Printf("failed to open package %s\n", pFilePath)
			return err
		}
		packages[uint32(i)] = f
		defer f.Close()
	}

	for k, v := range fullManifest.Frames {
		activeFile := packages[v.CurrentPackageIndex]
		activeFile.Seek(int64(v.CurrentOffset), 0)

		splitFile := make([]byte, v.CompressedSize)
		if v.CompressedSize == 0 {
			fmt.Println("skipping invalid entry with 0 compressed size")
			continue
		}
		_, err := io.ReadAtLeast(activeFile, splitFile, int(v.CompressedSize))

		if err != nil && v.DecompressedSize == 0 {
			fmt.Println("skipping invalid entry with 0 decompressed size")
			continue
		} else if err != nil {
			fmt.Println("failed to read file, check input")
			return err
		}

		fmt.Printf("Decompressing and extracting files contained in file index %d, %d/%d\n", k, totalFilesWritten, fullManifest.Header.FrameContents.Count)
		decompBytes, err := decompressZSTD(splitFile)
		if err != nil {
			return err
		}

		if len(decompBytes) != int(fullManifest.Frames[k].DecompressedSize) {
			return fmt.Errorf("size of decompressed data does not match manifest for file %d, is %d but should be %d", k, len(decompBytes), fullManifest.Frames[k].DecompressedSize)
		}

		for _, v2 := range fullManifest.FrameContents {
			if v2.FileIndex != uint32(k) {
				continue
			}
			fileName := strconv.FormatInt(v2.FileSymbol, 10)
			fileType := strconv.FormatInt(v2.T, 10)
			os.MkdirAll(fmt.Sprintf("%s/%s", outputDir, fileType), 0777)

			file, err := os.OpenFile(fmt.Sprintf("%s/%s/%s", outputDir, fileType, fileName), os.O_RDWR|os.O_CREATE, 0777)
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

func incrementHeaderChunk(chunk evrm.HeaderChunk, amount int) evrm.HeaderChunk {
	for i := 0; i < amount; i++ {
		chunk.Count++
		chunk.ElementCount++
		chunk.SectionSize += uint64(chunk.ElementSize)
	}
	return chunk
}

func writeManifest(manifest evrm.EvrManifest) error {
	os.MkdirAll(outputDir+"/manifests/", 0777)
	file, err := os.OpenFile(outputDir+"/manifests/"+packageName, os.O_RDWR|os.O_CREATE, 0777)
	if err != nil {
		return err
	}
	manifestBytes, err := evrm.UnmarshalManifest(manifest, manifestType)
	if err != nil {
		return err
	}
	file.Write(compressManifest(manifestBytes))
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
	fBuf.Write(zstdBytes)
	return fBuf.Bytes()
}
