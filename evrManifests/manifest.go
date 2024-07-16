package evrManifests

import "errors"

// evrManifest definition
type ManifestHeader struct {
	PackageCount  uint32
	Unk1          uint32 // ? - 524288 on latest builds
	Unk2          uint64 // ? - 0 on latest builds
	FrameContents HeaderChunk
	_             [16]byte // padding
	SomeStructure HeaderChunk
	_             [16]byte // padding
	Frames        HeaderChunk
}

type HeaderChunk struct {
	SectionSize  uint64 // total byte length of entire section
	Unk1         uint64 // ? 0 on latest builds
	Unk2         uint64 // ? 4294967296 on latest builds
	ElementSize  uint64 // byte size of single entry - TODO: confirm, only matches up with Frame_contents entry
	Count        uint64 // number of elements, can differ from ElementCount?
	ElementCount uint64 // number of elements
}

type FrameContents struct { // 32 bytes
	T             int64  // Probably filetype
	FileSymbol    int64  // Symbol for file
	FileIndex     uint32 // Frame[FileIndex] = file containing this entry
	DataOffset    uint32 // Byte offset for beginning of wanted data in given file
	Size          uint32 // Size of file
	SomeAlignment uint32 // file divisible by this (can this just be set to 1??) - yes
}

type SomeStructure struct { // 40 bytes
	T          int64 // seems to be the same as AssetType
	FileSymbol int64 // filename symbol
	Unk1       int64 // ? - game still launches when set to 0
	Unk2       int64 // ? - game still launches when set to 0
	AssetType  int64 // ? - game still launches when set to 0
}

type Frame struct { // 16 bytes
	CompressedSize        uint32 // compressed size of file
	DecompressedSize      uint32 // decompressed size of file
	NextEntryPackageIndex uint32 // the package index of the next entry
	NextEntryOffset       uint32 // the package byte offset of the next entry
}

type EvrManifest struct {
	Header        ManifestHeader
	FrameContents []FrameContents
	SomeStructure []SomeStructure
	_             [8]byte
	Frames        []Frame
}

// end evrManifest definition

// note: i have a sneaking suspicion that there's only one manifest version.
// the ones i've looked at so far can either be extracted by 5932408047-LE2 or 5932408047-EVR
// i think i remember being told this but i need to do more research

// every manifest version will be defined in it's own file
// each file should have functions to convert from evrManifest to it's type, and vice versa
// each file should also have a function to read and write itself to []byte

// this should take given manifestType and manifest []byte data, and call the appropriate function for that type, and return the result
func MarshalManifest(data []byte, manifestType string) (EvrManifest, error) {
	manifest := EvrManifest{}

	// switch based on manifestType
	switch manifestType {
	case "5932408047-LE2":
		m5932408047_LE2 := manifest_5932408047_LE2{}
		return m5932408047_LE2.evrmFromBytes(data)
	case "5932408047-EVR":
		m5932408047_EVR := manifest_5932408047_EVR{}
		return m5932408047_EVR.evrmFromBytes(data)
	case "5868485946-EVR":
		m5868485946_EVR := manifest_5868485946_EVR{}
		return m5868485946_EVR.evrmFromBytes(data)
	default:
		return manifest, errors.New("unimplemented manifest type")
	}
}

func UnmarshalManifest(m EvrManifest, manifestType string) ([]byte, error) {
	switch manifestType {
	case "5932408047-LE2":
		m5932408047_LE2 := manifest_5932408047_LE2{}
		return m5932408047_LE2.bytesFromEvrm(m)
	case "5932408047-EVR":
		m5932408047_EVR := manifest_5932408047_EVR{}
		return m5932408047_EVR.bytesFromEvrm(m)
	//case "5868485946-EVR":
	//	m5868485946_EVR := manifest_5868485946_EVR{}
	//	return m5868485946_EVR.bytesFromEvrm(m)
	default:
		return nil, errors.New("unimplemented manifest type")
	}
}
