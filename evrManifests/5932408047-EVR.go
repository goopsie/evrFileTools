package evrManifests

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// manifest structure ripped from Carnation, thank you exhibitmark <3
type manifest_5932408047_EVR struct {
	Header struct {
		PackageCount  uint32
		Unk1          uint32 // ? - 524288 on latest builds
		Unk2          uint64 // ? - 0 on latest builds
		FrameContents struct {
			SectionSize  uint64 // total byte length of entire section
			Unk1         uint64 // ? 0 on latest builds
			Unk2         uint64 // ? 4294967296 on latest builds
			ElementSize  uint64 // byte size of single entry - TODO: confirm, only matches up with Frame_contents entry
			Count        uint64 // number of elements, can differ from ElementCount?
			ElementCount uint64 // number of elements
		}
		_             [16]byte // padding
		SomeStructure struct {
			SectionSize  uint64 // total byte length of entire section
			Unk1         uint64 // ? 0 on latest builds
			Unk2         uint64 // ? 4294967296 on latest builds
			ElementSize  uint64 // byte size of single entry - TODO: confirm, only matches up with Frame_contents entry
			Count        uint64 // number of elements, can differ from ElementCount?
			ElementCount uint64 // number of elements
		}
		_      [16]byte // padding
		Frames struct {
			SectionSize  uint64 // total byte length of entire section
			Unk1         uint64 // ? 0 on latest builds
			Unk2         uint64 // ? 4294967296 on latest builds
			ElementSize  uint64 // byte size of single entry - TODO: confirm, only matches up with Frame_contents entry
			Count        uint64 // number of elements, can differ from ElementCount?
			ElementCount uint64 // number of elements
		}
	}
	FrameContents []struct {
		TypeSymbol    int64  // Probably filetype
		FileSymbol    int64  // Symbol for file
		FileIndex     uint32 // Frame[FileIndex] = file containing this entry
		DataOffset    uint32 // Byte offset for beginning of wanted data in given file
		Size          uint32 // Size of file
		SomeAlignment uint32 // file divisible by this (can this just be set to 1??) - yes
	}
	SomeStructure []struct {
		TypeSymbol int64 // seems to be the same as unk3 (for a few files on quest, at least)
		FileSymbol int64 // filename symbol
		Unk1       int64 // ? - game still launches when set to 0
		Unk2       int64 // ? - game still launches when set to 0
		Unk3       int64 // ? - game still launches when set to 0
	}
	Frames []struct {
		CurrentPackageIndex uint32 // the package index
		CurrentOffset       uint32 // the package byte offset
		CompressedSize      uint32 // compressed size of file
		DecompressedSize    uint32 // decompressed size of file
	}
}

func (m *manifest_5932408047_EVR) evrmFromBytes(b []byte) (EvrManifest, error) {
	newManifest := EvrManifest{}
	if err := m.unmarshalManifest(b); err != nil {
		return newManifest, err
	}

	return m.convToEvrm()
}

func (m *manifest_5932408047_EVR) bytesFromEvrm(evrm EvrManifest) ([]byte, error) {
	if err := m.evrmToOrig(evrm); err != nil {
		return nil, err
	}

	wbuf := bytes.NewBuffer(nil)

	var data = []any{
		m.Header,
		m.FrameContents,
		m.SomeStructure,
		m.Frames,
	}
	for _, v := range data {
		err := binary.Write(wbuf, binary.LittleEndian, v)
		if err != nil {
			fmt.Println("binary.Write failed:", err)
		}
	}

	manifestBytes := wbuf.Bytes()
	return manifestBytes, nil // hack
}

func (m *manifest_5932408047_EVR) convToEvrm() (EvrManifest, error) {
	newManifest := EvrManifest{
		Header: ManifestHeader{
			PackageCount:  m.Header.PackageCount,
			Unk1:          m.Header.Unk1,
			Unk2:          m.Header.Unk2,
			FrameContents: m.Header.FrameContents,
			SomeStructure: m.Header.SomeStructure,
			Frames:        m.Header.Frames,
		},
		FrameContents: make([]FrameContents, len(m.FrameContents)),
		SomeStructure: make([]SomeStructure, len(m.SomeStructure)),
		Frames:        make([]Frame, len(m.Frames)),
	}
	for k, v := range m.FrameContents {
		newManifest.FrameContents[k] = FrameContents{
			T:             v.TypeSymbol,
			FileSymbol:    v.FileSymbol,
			FileIndex:     v.FileIndex,
			DataOffset:    v.DataOffset,
			Size:          v.Size,
			SomeAlignment: v.SomeAlignment,
		}
	}
	for k, v := range m.SomeStructure {
		newManifest.SomeStructure[k] = SomeStructure{
			T:          v.TypeSymbol,
			FileSymbol: v.FileSymbol,
			Unk1:       v.Unk1,
			Unk2:       v.Unk2,
			AssetType:  v.Unk3,
		}
	}
	for k, v := range m.Frames {
		newManifest.Frames[k] = Frame{
			CurrentPackageIndex: v.CurrentPackageIndex,
			CurrentOffset:       v.CurrentOffset,
			CompressedSize:      v.CompressedSize,
			DecompressedSize:    v.DecompressedSize,
		}
	}
	return newManifest, nil
}

func (m *manifest_5932408047_EVR) evrmToOrig(evrm EvrManifest) error {
	m.Header = struct {
		PackageCount  uint32
		Unk1          uint32
		Unk2          uint64
		FrameContents struct {
			SectionSize  uint64
			Unk1         uint64
			Unk2         uint64
			ElementSize  uint64
			Count        uint64
			ElementCount uint64
		}
		_             [16]byte
		SomeStructure struct {
			SectionSize  uint64
			Unk1         uint64
			Unk2         uint64
			ElementSize  uint64
			Count        uint64
			ElementCount uint64
		}
		_      [16]byte
		Frames struct {
			SectionSize  uint64
			Unk1         uint64
			Unk2         uint64
			ElementSize  uint64
			Count        uint64
			ElementCount uint64
		}
	}{
		PackageCount:  evrm.Header.PackageCount,
		Unk1:          evrm.Header.Unk1,
		Unk2:          evrm.Header.Unk2,
		FrameContents: evrm.Header.FrameContents,
		SomeStructure: evrm.Header.SomeStructure,
		Frames:        evrm.Header.Frames,
	}

	m.FrameContents = make([]struct {
		TypeSymbol    int64
		FileSymbol    int64
		FileIndex     uint32
		DataOffset    uint32
		Size          uint32
		SomeAlignment uint32
	}, len(evrm.FrameContents))

	m.SomeStructure = make([]struct {
		TypeSymbol int64
		FileSymbol int64
		Unk1       int64
		Unk2       int64
		Unk3       int64
	}, len(evrm.SomeStructure))

	m.Frames = make([]struct {
		CurrentPackageIndex uint32
		CurrentOffset       uint32
		CompressedSize      uint32
		DecompressedSize    uint32
	}, len(evrm.Frames))

	for k, v := range evrm.FrameContents {
		m.FrameContents[k] = struct {
			TypeSymbol    int64
			FileSymbol    int64
			FileIndex     uint32
			DataOffset    uint32
			Size          uint32
			SomeAlignment uint32
		}{
			TypeSymbol:    v.T,
			FileSymbol:    v.FileSymbol,
			FileIndex:     v.FileIndex,
			DataOffset:    v.DataOffset,
			Size:          v.Size,
			SomeAlignment: v.SomeAlignment,
		}
	}

	for k, v := range evrm.SomeStructure {
		m.SomeStructure[k] = struct {
			TypeSymbol int64
			FileSymbol int64
			Unk1       int64
			Unk2       int64
			Unk3       int64
		}{
			TypeSymbol: v.T,
			FileSymbol: v.FileSymbol,
			Unk1:       v.Unk1,
			Unk2:       v.Unk2,
			Unk3:       v.AssetType,
		}
	}

	for k, v := range evrm.Frames {
		m.Frames[k] = struct {
			CurrentPackageIndex uint32
			CurrentOffset       uint32
			CompressedSize      uint32
			DecompressedSize    uint32
		}{
			CurrentPackageIndex: v.CurrentPackageIndex,
			CurrentOffset:       v.CurrentOffset,
			CompressedSize:      v.CompressedSize,
			DecompressedSize:    v.DecompressedSize,
		}
	}

	return nil
}

func (m *manifest_5932408047_EVR) unmarshalManifest(b []byte) error {
	currentOffset := binary.Size(m.Header)
	buf := bytes.NewReader(b[:currentOffset])
	if err := binary.Read(buf, binary.LittleEndian, &m.Header); err != nil {
		return err
	}

	m.FrameContents = make([]struct {
		TypeSymbol    int64
		FileSymbol    int64
		FileIndex     uint32
		DataOffset    uint32
		Size          uint32
		SomeAlignment uint32
	}, m.Header.FrameContents.ElementCount)
	m.SomeStructure = make([]struct {
		TypeSymbol int64
		FileSymbol int64
		Unk1       int64
		Unk2       int64
		Unk3       int64
	}, m.Header.SomeStructure.ElementCount)
	m.Frames = make([]struct {
		CurrentPackageIndex uint32
		CurrentOffset       uint32
		CompressedSize      uint32
		DecompressedSize    uint32
	}, m.Header.Frames.ElementCount)

	buf = bytes.NewReader(b[currentOffset : currentOffset+binary.Size(m.FrameContents)])
	if err := binary.Read(buf, binary.LittleEndian, &m.FrameContents); err != nil {
		return err
	}
	currentOffset += binary.Size(m.FrameContents)

	buf = bytes.NewReader(b[currentOffset : currentOffset+binary.Size(m.SomeStructure)])
	if err := binary.Read(buf, binary.LittleEndian, &m.SomeStructure); err != nil {
		return err
	}
	currentOffset += binary.Size(m.SomeStructure)

	buf = bytes.NewReader(b[currentOffset : currentOffset+binary.Size(m.Frames)])
	if err := binary.Read(buf, binary.LittleEndian, &m.Frames); err != nil {
		return err
	}

	return nil
}
