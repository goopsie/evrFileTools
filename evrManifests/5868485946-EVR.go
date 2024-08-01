package evrManifests

import (
	"bytes"
	"encoding/binary"
)

type manifest_5868485946_EVR struct {
	Header struct {
		PackageCount  uint32
		Unk1          uint32
		Unk2          uint64
		_             [8]byte
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
	}
	FrameContents []struct {
		TypeSymbol    int64
		FileSymbol    int64
		FileIndex     uint32
		DataOffset    uint32
		Size          uint32
		SomeAlignment uint32
	}
	SomeStructure []struct {
		TypeSymbol int64
		FileSymbol int64
		Unk1       int64
		Unk2       int64
		Unk3       uint32
		Unk4       uint32
	}
	Frames []struct {
		CurrentPackageIndex uint32
		CurrentOffset       uint32
		CompressedSize      uint32
		DecompressedSize    uint32
	}
}

func (m *manifest_5868485946_EVR) evrmFromBytes(b []byte) (EvrManifest, error) {
	newManifest := EvrManifest{}
	if err := m.unmarshalManifest(b); err != nil {
		return newManifest, err
	}

	return m.convToEvrm()
}

func (m *manifest_5868485946_EVR) convToEvrm() (EvrManifest, error) {
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
		// combine Unk3 and Unk4 into one uint64 and place in AssetType
		atBytes := (int64(v.Unk3) << 32) | int64(v.Unk4) // autogenerated, i'm scared of this
		newManifest.SomeStructure[k] = SomeStructure{
			T:          v.TypeSymbol,
			FileSymbol: v.FileSymbol,
			Unk1:       v.Unk1,
			Unk2:       v.Unk2,
			AssetType:  atBytes,
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

func (m *manifest_5868485946_EVR) unmarshalManifest(b []byte) error {
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
		Unk3       uint32
		Unk4       uint32
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
