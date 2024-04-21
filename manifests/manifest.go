package manifests

// practically a 1:1 rip from carnation
// thank you exhibitmark for doing all of the hard work

type CompressedHeader struct {
	Magic            [4]byte
	HeaderSize       uint32
	UncompressedSize uint64
	CompressedSize   uint64
}

type Manifest_header struct {
	PackageCount uint32
	Unk1         uint32 // ? - 524288 on latest builds
	Unk2         uint64 // ? - 0 on latest builds
	// _             [8]byte // padding - for le2 only
	FrameContents Header_chunk
	_             [16]byte // padding
	SomeStructure Header_chunk
	_             [16]byte // padding
	Frames        Header_chunk
}

type Header_chunk struct {
	SectionSize  uint64 // total byte length of entire section
	Unk1         uint64 // ? 0 on latest builds
	Unk2         uint64 // ? 4294967296 on latest builds
	ElementSize  uint64 // byte size of single entry - TODO: confirm, only matches up with Frame_contents entry
	Count        uint64 // number of elements, can differ from ElementCount?
	ElementCount uint64 // number of elements
}

type Frame_contents struct { // 32 bytes
	T             int64  // Probably filetype
	FileSymbol    int64  // Symbol for file
	FileIndex     uint32 // Frame[FileIndex] = file containing this entry
	DataOffset    uint32 // Byte offset for beginning of wanted data in given file
	Size          uint32 // Size of file
	SomeAlignment uint32 // file divisible by this (can this just be set to 1??) - yes
}

type Some_structure1 struct { // 40 bytes
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

type FullManifest struct { // v5932408047
	Header        Manifest_header
	FrameContents []Frame_contents
	SomeStructure []Some_structure1
	_             [8]byte // [8]byte{0x00...}, on latest builds, padding?
	Frames        []Frame
}
