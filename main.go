package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"

	"github.com/klauspost/compress/zstd"
)

// practically a 1:1 rip from carnation
// thank you exhibitmark for doing all of the hard work

type CompressedHeader struct { // can recreate
	Magic            [4]byte
	HeaderSize       uint32
	UncompressedSize uint64
	CompressedSize   uint64
}

type Manifest_header struct { // can't recreate
	PackageCount uint32
	Unk1         uint32 // ?
	Unk2         uint64 // ?
	Section1     Header_chunk
	_            [16]byte // padding
	Section2     Header_chunk
	_            [16]byte // padding
	Section3     Header_chunk
}

type Header_chunk struct { // can't recreate
	SectionSize  uint64
	Unk1         [16]byte // ?
	ElementSize  uint64   // size in bytes
	Count        uint64   // number of elements
	ElementCount uint64   // number of elements
}

type Frame_contents struct { // can recreate
	T             int64  // Probably filetype
	FileSymbol    int64  // Symbol for file
	FileIndex     uint32 // Frame[FileIndex] = file containing this entry
	DataOffset    uint32 // Byte offset for beginning of wanted data in given file
	Size          uint32 // Size of file
	SomeAlignment uint32 // file divisible by this (could we get away by setting this to 1?)
}

type Some_structure1 struct { // can't recreate
	T          int64 // seems to be the same as AssetType
	FileSymbol int64 // filename symbol
	Unk1       int64 // ? - nothing seems to change when set to 0, seems fine
	Unk2       int64 // ? - nothing seems to change when set to 0, seems fine
	AssetType  int64 // ? - name from carnation, unknown atm
}

type Frame struct { // can recreate
	CompressedSize        uint32 // compressed size of file
	DecompressedSize      uint32 // decompressed size of file
	NextEntryPackageIndex uint32 // the package index of the next entry
	NextEntryOffset       uint32 // the data offset of the next entry
}

type FullManifest struct { // v5932408047
	Header        Manifest_header
	FrameContents []Frame_contents
	Unk1          []Some_structure1
	Unk2          []byte
	FileInfo      []Frame
	Unk3          []byte
}

// end of carnation plagiarism

type ModifiedFile struct { // This is for an individual decompressed file
	TypeSymbol       int64
	FileSymbol       int64
	ModifiedFilePath string
	PackageIndex     uint32
	DataOffset       uint32
	FileSize         uint32
}

func main() {
	mode := flag.String("mode", "", "Either 'extract' or 'replace'")
	packageName := flag.String("packageName", "", "File name of package, e.g. 48037dc70b0ecab2, 2b47aab238f60515, etc")
	dataDir := flag.String("dataDir", "", "Path of directory containing 'manifests' & 'packages' in ready-at-dawn-echo-arena/_data")
	modifiedFolder := flag.String("modifiedFolder", "", "Path of directory containing modified files (same structure as '-mode extract' output)")
	outputFolder := flag.String("outputFolder", "", "Path of directory to place modified package & manifest files")
	help := flag.Bool("help", false, "Print usage")
	flag.Parse()

	if *help || len(os.Args) == 1 || (*packageName) == "" || (*dataDir) == "" || (*mode) == "" || (*outputFolder) == "" {
		flag.Usage()
		return
	}

	if (*mode) != "extract" && (*mode) != "replace" {
		fmt.Println("mode must be either 'extract' or 'replace'")
		flag.Usage()
		return
	}

	if *(mode) == "replace" && (*modifiedFolder) == "" {
		fmt.Println("'replace' must be used in conjunction with '-modifiedFolder'")
		flag.Usage()
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
		if err := whatincarnation(manifest, (*dataDir), (*packageName), (*outputFolder)); err != nil {
			fmt.Println(err)
		}
		return
	}

	fmt.Println("Scanning folder for modified files, might take a while depending on input...")
	modifiedFiles, err := scanModifiedFiles(manifest, (*modifiedFolder))
	if err != nil {
		fmt.Printf("failed to scan %s", (*modifiedFolder))
		panic(err)
	}

	for k, v := range modifiedFiles {
		for k2, v2 := range v {
			fmt.Printf("Modified file %d, Entry %d information: \n", k, k2)
			fmt.Printf("	Filetype symbol: %d\n", v2.TypeSymbol)
			fmt.Printf("	File symbol: %d\n", v2.FileSymbol)
			fmt.Printf("	Filepath: %s\n", v2.ModifiedFilePath)
			fmt.Printf("	Package index: %d\n", v2.PackageIndex)
			fmt.Printf("	File data offset: %d\n", v2.DataOffset)
			fmt.Printf("	Filesize: %d\n", v2.FileSize)
		}
	}

	if err := doEverything((*outputFolder), (*dataDir), manifest, modifiedFiles, (*packageName)); err != nil {
		fmt.Println(err)
	}
}

func whatincarnation(fullManifest FullManifest, dataDir string, packageName string, outputFolder string) error {
	files, err := splitPackages(fullManifest, dataDir, packageName)
	if err != nil {
		fmt.Println("failed to split package, check input")
		return err
	}
	for _, v := range fullManifest.FrameContents {
		fileName := strconv.FormatInt(v.FileSymbol, 10)
		fileType := strconv.FormatInt(v.T, 10)
		fmt.Printf("Extracting %s:%s\n", fileType, fileName)
		os.MkdirAll(fmt.Sprintf("%s/%s", outputFolder, fileType), 0777)

		file, err := os.OpenFile(fmt.Sprintf("%s/%s/%s", outputFolder, fileType, fileName), os.O_RDWR|os.O_CREATE, 0777)
		if err != nil {
			fmt.Println(err)
			continue
		}
		decompBytes, err := decompressZSTD(files[v.FileIndex])
		if err != nil {
			return err
		}
		file.Write(decompBytes[v.DataOffset : v.DataOffset+v.Size])
		file.Close()
	}
	return nil
}

func doEverything(outputFolder string, dataDir string, manifest FullManifest, modifiedFiles map[uint32][]ModifiedFile, packageName string) error {
	// need a better name for this
	// i wrote most of this in one sitting, please forgive anything stupid

	fmt.Println("Reading required files into memory")
	filesSplitByIndex, err := splitPackages(manifest, dataDir, packageName)
	if err != nil {
		fmt.Println("failed to split package file(s)")
		return err
	}

	newFiles := map[uint32][]byte{}
	newManifest := manifest
	// Loop through every file provided from scanModifiedFiles
	// Reconstruct x.bin from original files, replacing with our own when symbols match
	// Modify manifest entry when replacing file
	for k, v := range modifiedFiles {
		fmt.Printf("Modifying manifest for file %d\n", k)
		originalFileInfo := []Frame_contents{}
		for _, v2 := range manifest.FrameContents {
			if v2.FileIndex != k {
				continue
			}
			originalFileInfo = append(originalFileInfo, v2)
		}
		// i remember that there *was* a reason we sorted but i can't remember what it was and i'm just going to trust past me knew what he was doing
		sort.Slice(originalFileInfo, func(i, j int) bool { return originalFileInfo[i].DataOffset < originalFileInfo[j].DataOffset })
		decompBytes, err := decompressZSTD(filesSplitByIndex[k])
		if err != nil {
			return err
		}
		for _, entry := range originalFileInfo {
			foo := decompBytes[entry.DataOffset : entry.DataOffset+entry.Size]
			for _, newentry := range v { // bad
				if entry.T != newentry.TypeSymbol {
					continue
				}
				if entry.FileSymbol != newentry.FileSymbol {
					continue
				}
				foo, _ = os.ReadFile(newentry.ModifiedFilePath)
				modifyManifestEntry(&newManifest, newentry.TypeSymbol, entry.FileSymbol, (len(foo) - int(entry.Size)))
			}
			newFiles[k] = append(newFiles[k], foo...)
		}
	}

	if _, err := os.Stat(outputFolder + "/packages/"); os.IsNotExist(err) {
		os.MkdirAll(outputFolder+"/packages/", 0777)
	}
	if _, err := os.Stat(outputFolder + "/manifests/"); os.IsNotExist(err) {
		os.MkdirAll(outputFolder+"/manifests/", 0777)
	}

	packages := make(map[uint32]*os.File)

	for i := 0; i < int(manifest.Header.PackageCount); i++ {
		pFilePath := fmt.Sprintf("%s/packages/%s_%d", outputFolder, packageName, i)
		f, err := os.OpenFile(pFilePath, os.O_RDWR|os.O_CREATE, 0777)
		if err != nil {
			fmt.Printf("failed to open package %s\n", pFilePath)
			return err
		}
		if err := os.Truncate(f.Name(), 0); err != nil {
			fmt.Printf("failed to truncate: %v\n", err)
			return err
		}

		packages[uint32(i)] = f
		defer f.Close()
	}
	activeFile := packages[0]
	filesSplitByIndexLen := uint32(len(filesSplitByIndex))
	for k := uint32(0); k < filesSplitByIndexLen; k++ {
		pos, _ := activeFile.Seek(0, 1)
		if bytes.Equal(newFiles[k], []byte{}) {
			if len(filesSplitByIndex[k]) == 0 && k > 0 { // broken file that should be removed
				// holy moly do i not like this
				fmt.Printf("skipping empty file %d/%d\n", k, len(filesSplitByIndex))
				// modify newManifest.FileInfo[k-1] to have NextEntryOffset & NextEntryPackageIndex that of newManifest.FileInfo[k], and remove newManifest.FileInfo[k]
				fmt.Println("removing broken entry from manifest")
				newManifest.FileInfo[k-1].NextEntryOffset = newManifest.FileInfo[k].NextEntryOffset
				newManifest.FileInfo[k-1].NextEntryPackageIndex = newManifest.FileInfo[k].NextEntryPackageIndex
				newManifest.Header.Section3.ElementCount--
				newManifest.Header.Section3.SectionSize = newManifest.Header.Section3.SectionSize - 16
				// at this point every entry trying to index files >k will be broken, so decrement them by one
				for i := 0; i < len(newManifest.FrameContents); i++ {
					if newManifest.FrameContents[i].FileIndex <= k {
						continue
					}
					newManifest.FrameContents[i].FileIndex--
				}
				// we need to remove the faulty entry from newManifest and filesSplitByIndex
				newManifest.FileInfo = append(newManifest.FileInfo[:k], newManifest.FileInfo[k+1:]...)
				// loop over filesSplitByIndex and offset every entry
				for i := k; i < uint32(len(filesSplitByIndex)); i++ {
					if i == uint32(len(filesSplitByIndex)) {
						filesSplitByIndexLen--
						delete(filesSplitByIndex, i)
						break
					}
					filesSplitByIndexLen--
					filesSplitByIndex[i] = filesSplitByIndex[i+1]
					delete(filesSplitByIndex, i+1)
				}

				k--
				continue
			}
			fmt.Printf("Writing file %d to package %s, length %d\n", k, activeFile.Name(), len(filesSplitByIndex[k]))
			numBytesWritten, err := activeFile.Write(filesSplitByIndex[k])
			if err != nil {
				fmt.Printf("failed to write file %d, only wrote %d bytes\n", k, numBytesWritten)
				return err
			}
		} else {
			fmt.Printf("Writing file %d to package %s, length %d\n", k, activeFile.Name(), len(newFiles[k]))
			fmt.Printf("Compressing and writing modified file %d to disk, offset %d\n", k, pos)
			compFile, err := compressZSTD(newFiles[k])
			if err != nil {
				fmt.Printf("failed to compress file %d\n", k)
				return err
			}
			numBytesWritten, err := activeFile.Write(compFile)
			if err != nil {
				fmt.Printf("failed to write file %d, only wrote %d bytes\n", k, numBytesWritten)
				return err
			}
			fmt.Printf("	Modifying fileInfo manifest entries for file %d\n", k)
			modifyManifestFileInfoEntry(
				&newManifest,
				uint32(len(compFile)-len(filesSplitByIndex[k])),
				uint32(len(newFiles[k]))-newManifest.FileInfo[k].DecompressedSize,
				k,
			)
		}

		activeFile = packages[newManifest.FileInfo[k].NextEntryPackageIndex]
		activeFile.Seek(int64(newManifest.FileInfo[k].NextEntryOffset), 0)
	}
	fmt.Println("Setting SomeStructure1's unk1 & unk2 to 0")

	for i := 0; i < len(newManifest.Unk1); i++ {
		newManifest.Unk1[i].Unk1 = 0
		newManifest.Unk1[i].Unk2 = 0
	}

	fmt.Println("Writing modified manifest...")
	if err := writeManifest(newManifest, outputFolder, packageName); err != nil {
		fmt.Println("error writing manifest")
		return err
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
		splitFile := make([]byte, v.CompressedSize)
		numBytesRead, err := io.ReadAtLeast(activeFile, splitFile, int(v.CompressedSize))
		if err != nil {
			if numBytesRead != int(v.CompressedSize) {
				fmt.Printf("only read %d bytes while reading file %d, expected %d, encountered error\n", numBytesRead, k, v.CompressedSize)
				// loop over manifest.FrameInfo to see if file exists
				// this effectively only skips files 10301 and 10302
				// ideally we'd want to preserve these but i'm at my wit's end regarding this
				for _, frame := range manifest.FrameContents {
					if frame.FileIndex == uint32(k) {
						fmt.Printf("failed to read file %d with manifest entry\n", k)
						fmt.Println("this should only happen for 10301 and 10302, check input otherwise")
						return nil, err
					}
				}
				fmt.Println("skipping file with no manifest entry")
				splitFile = []byte{}
			} else {
				fmt.Printf("failed to read file %d\n", k)
				return nil, err
			}
		}
		splitFiles[uint32(k)] = splitFile

		activeFile = packages[v.NextEntryPackageIndex]
		activeFile.Seek(int64(v.NextEntryOffset), 0)
	}

	return splitFiles, nil
}

func scanModifiedFiles(manifest FullManifest, modifiedFolder string) (map[uint32][]ModifiedFile, error) {
	pFileTypes, err := os.ReadDir(modifiedFolder)
	if err != nil {
		fmt.Printf("failed to read from %s\n", modifiedFolder)
		return nil, err
	}
	modifiedFiles := make(map[uint32][]ModifiedFile)
	// os.Walk maybe?
	// get working first before changing it, don't need to be working on this part right now
	for _, v := range pFileTypes {
		currentType := v.Name()
		currentDir := modifiedFolder + "/" + currentType
		pFiles, err := os.ReadDir(currentDir)
		if err != nil {
			fmt.Printf("failed to read directory %s, check input\n", currentType)
			return nil, err
		}
		for _, v2 := range pFiles {
			currentFile := currentDir + ("/" + v2.Name())
			fInfo, err := os.Stat(currentFile)
			if err != nil {
				fmt.Printf("failed to read %s, check input\n", currentFile)
				return nil, err
			}

			typeSymbol, err := strconv.ParseInt(v.Name(), 10, 0)
			if err != nil {
				fmt.Printf("failed to convert %s to int, skipping\n", v.Name())
				continue
			}
			fileSymbol, err := strconv.ParseInt(v2.Name(), 10, 0)
			if err != nil {
				fmt.Printf("failed to convert %s to int, skipping\n", v2.Name())
				continue
			}
			packageIndex := uint32(0)
			fileIndex := uint32(0)
			fileDataOffset := uint32(0)
			for _, v := range manifest.FrameContents {
				if v.T != typeSymbol || v.FileSymbol != fileSymbol {
					continue
				}
				fileIndex = v.FileIndex
				packageIndex = manifest.FileInfo[v.FileIndex].NextEntryPackageIndex
				fileDataOffset = v.DataOffset
				if v.FileIndex > 0 {
					packageIndex = manifest.FileInfo[v.FileIndex-1].NextEntryPackageIndex
				}
			}
			entry := ModifiedFile{}

			entry.TypeSymbol = typeSymbol
			entry.FileSymbol = fileSymbol
			entry.ModifiedFilePath = currentFile
			entry.PackageIndex = packageIndex
			entry.DataOffset = fileDataOffset
			entry.FileSize = uint32(fInfo.Size())

			modifiedFiles[fileIndex] = append(modifiedFiles[fileIndex], entry)
		}
	}
	return modifiedFiles, nil
}

func modifyManifestEntry(manifest *FullManifest, fileType int64, fileSymbol int64, fileSizeDifference int) {
	// Loop through FrameContents until FileIndex = fileIndexToModify.
	// Edit every entry where DataOffset is > fileDataOffset (File to be modified)
	// Adjust DataOffset of current file to match arguments
	// Adjust DataOffset of every subsequent file to align based on fileSizeDifference

	// Todo: rewrite logic to work directly with symbols instead of keeping this hack around

	fileDataOffset := uint32(0)
	fileIndexToModify := 0

	for _, v := range manifest.FrameContents {
		if v.T != fileType || v.FileSymbol != fileSymbol {
			continue
		}
		fileDataOffset = v.DataOffset
		fileIndexToModify = int(v.FileIndex)
		break
	}

	for k, v := range manifest.FrameContents {
		if v.FileIndex == uint32(fileIndexToModify) {
			if v.DataOffset > uint32(fileDataOffset) {
				manifest.FrameContents[k].DataOffset = uint32(int(manifest.FrameContents[k].DataOffset) + fileSizeDifference)
			} else if v.DataOffset == uint32(fileDataOffset) {
				manifest.FrameContents[k].Size = uint32(int(manifest.FrameContents[k].Size) + fileSizeDifference)
			}
		}
	}
}

func modifyManifestFileInfoEntry(manifest *FullManifest, zstdSizeDifference uint32, fileSizeDifference uint32, fileIndexToModify uint32) {
	// Loop through every entry in FileInfo in the same package as fileIndexToModify until at FileInfo[fileIndexToModify]
	// Adjust compressed & uncompressed size of entry to match arguments
	// Adjust every subsequent entry's NextOffset to match arguments
	frameOffset := uint32(0)
	if fileIndexToModify > 0 {
		frameOffset = 1
	}
	for i := fileIndexToModify; i < uint32(len(manifest.FileInfo)); i++ {
		if manifest.FileInfo[i-frameOffset].NextEntryPackageIndex != manifest.FileInfo[fileIndexToModify-frameOffset].NextEntryPackageIndex {
			continue
		}
		if i == fileIndexToModify {
			manifest.FileInfo[i].CompressedSize = manifest.FileInfo[i].CompressedSize + zstdSizeDifference
			manifest.FileInfo[i].DecompressedSize = manifest.FileInfo[i].DecompressedSize + fileSizeDifference
			if manifest.FileInfo[i].NextEntryOffset != 0 {
				manifest.FileInfo[i].NextEntryOffset = manifest.FileInfo[i].NextEntryOffset + zstdSizeDifference
			}
			continue
		}

		if manifest.FileInfo[i].NextEntryPackageIndex == manifest.FileInfo[i-frameOffset].NextEntryPackageIndex {
			manifest.FileInfo[i].NextEntryOffset = manifest.FileInfo[i].NextEntryOffset + zstdSizeDifference
		}
	}
}

func unmarshalManifest(b []byte) FullManifest {
	// copied straight from me three weeks ago, i'm not making this look nicer
	currentOffset := 192
	manifestHeader := Manifest_header{}
	buf := bytes.NewReader(b[:currentOffset]) // ideally this shouldn't be hardcoded but for now it's fine
	err := binary.Read(buf, binary.LittleEndian, &manifestHeader)
	if err != nil {
		panic(err)
	}

	frameContents := make([]Frame_contents, manifestHeader.Section1.ElementCount)
	frameContentsLength := binary.Size(frameContents)
	frameContentsBuf := bytes.NewReader(b[currentOffset : currentOffset+frameContentsLength])
	currentOffset = currentOffset + frameContentsLength
	if err := binary.Read(frameContentsBuf, binary.LittleEndian, &frameContents); err != nil {
		fmt.Println("Failed to marshal frameContents into struct")
		panic(err)
	}

	someStructure1 := make([]Some_structure1, manifestHeader.Section2.ElementCount)
	someStructure1Length := binary.Size(someStructure1)
	someStructure1Buf := bytes.NewReader(b[currentOffset : currentOffset+someStructure1Length])
	currentOffset = currentOffset + someStructure1Length
	if err := binary.Read(someStructure1Buf, binary.LittleEndian, &someStructure1); err != nil {
		fmt.Println("Failed to marshal someStructure1 into struct")
		panic(err)
	}

	unk2 := b[currentOffset : currentOffset+8]
	currentOffset = currentOffset + 8

	manifestFileInfo := make([]Frame, manifestHeader.Section3.ElementCount-1)
	manifestFileInfoLength := binary.Size(manifestFileInfo)
	manifestFileInfoBuf := bytes.NewReader(b[currentOffset : currentOffset+manifestFileInfoLength])
	if err := binary.Read(manifestFileInfoBuf, binary.LittleEndian, &manifestFileInfo); err != nil {
		fmt.Println("Failed to marshal manifestFileInfo into struct")
		panic(err)
	}

	fullManifest := FullManifest{
		manifestHeader,
		frameContents,
		someStructure1,
		unk2,
		manifestFileInfo,
		b[len(b)-8:],
	}

	return fullManifest
}

func decompressZSTD(b []byte) ([]byte, error) {
	decoder, _ := zstd.NewReader(nil)
	decomp, err := decoder.DecodeAll(b, nil)
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
		manifest.Unk1,
		manifest.Unk2,
		manifest.FileInfo,
		manifest.Unk3,
	}
	for _, v := range data {
		err := binary.Write(wbuf, binary.LittleEndian, v)
		if err != nil {
			fmt.Println("binary.Write failed:", err)
		}
	}

	os.MkdirAll(outputFolder+"/manifests/", 0777)
	file, err := os.OpenFile(outputFolder+"/manifests/"+packageName, os.O_RDWR|os.O_CREATE, 0777)
	if err != nil {
		return err
	}
	file.Write(compressManifest(wbuf.Bytes()))
	file.Close()
	return nil
}

func compressManifest(b []byte) []byte {
	zstdBytes, err := compressZSTD(b)
	if err != nil {
		fmt.Println("failed to compress manifest")
		panic(err)
	}
	cHeader := CompressedHeader{
		[4]byte{0x5A, 0x53, 0x54, 0x44}, // Z S T D
		16,
		uint64(len(b)),
		uint64(len(zstdBytes)),
	}

	fBuf := bytes.NewBuffer(nil)
	binary.Write(fBuf, binary.LittleEndian, cHeader)

	return append(fBuf.Bytes(), zstdBytes...)
}

func compressZSTD(b []byte) ([]byte, error) {
	enc, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBestCompression)) // sometimes this script crashes when decompressing this data, maybe an issue with zstd library? dunno
	if err != nil {
		return nil, err
	}
	zstdBytes := enc.EncodeAll(b, nil)
	enc.Close()
	return zstdBytes, nil
}
