package gobackend

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// AudioMetadata represents common audio file metadata
type AudioMetadata struct {
	Title       string
	Artist      string
	Album       string
	AlbumArtist string
	Genre       string
	Year        string
	Date        string
	TrackNumber int
	DiscNumber  int
	ISRC        string
}

// MP3Quality represents MP3 specific quality info
type MP3Quality struct {
	SampleRate int
	BitDepth   int
	Duration   int
	Bitrate    int
}

// OggQuality represents Ogg/Opus specific quality info
type OggQuality struct {
	SampleRate int
	BitDepth   int
	Duration   int
}

// =============================================================================
// ID3 Tag Reading (MP3)
// =============================================================================

// ReadID3Tags reads ID3v2 and ID3v1 tags from an MP3 file
func ReadID3Tags(filePath string) (*AudioMetadata, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	metadata := &AudioMetadata{}

	// Try ID3v2 first (at beginning of file)
	id3v2, err := readID3v2(file)
	if err == nil && id3v2 != nil {
		metadata = id3v2
	}

	// If ID3v2 failed or is incomplete, try ID3v1 (at end of file)
	if metadata.Title == "" || metadata.Artist == "" {
		id3v1, err := readID3v1(file)
		if err == nil && id3v1 != nil {
			// Fill in missing fields
			if metadata.Title == "" {
				metadata.Title = id3v1.Title
			}
			if metadata.Artist == "" {
				metadata.Artist = id3v1.Artist
			}
			if metadata.Album == "" {
				metadata.Album = id3v1.Album
			}
			if metadata.Year == "" {
				metadata.Year = id3v1.Year
			}
			if metadata.Genre == "" {
				metadata.Genre = id3v1.Genre
			}
		}
	}

	if metadata.Title == "" && metadata.Artist == "" {
		return nil, fmt.Errorf("no ID3 tags found")
	}

	return metadata, nil
}

// readID3v2 reads ID3v2 tags from the beginning of file
func readID3v2(file *os.File) (*AudioMetadata, error) {
	file.Seek(0, io.SeekStart)

	// Read ID3v2 header (10 bytes)
	header := make([]byte, 10)
	if _, err := io.ReadFull(file, header); err != nil {
		return nil, err
	}

	// Check for "ID3" identifier
	if string(header[0:3]) != "ID3" {
		return nil, fmt.Errorf("no ID3v2 header")
	}

	// Get version
	majorVersion := header[3]
	// minorVersion := header[4]
	flags := header[5]
	unsync := (flags & 0x80) != 0
	extendedHeader := (flags & 0x40) != 0
	footerPresent := (flags & 0x10) != 0

	// Get tag size (syncsafe integer)
	size := int(header[6])<<21 | int(header[7])<<14 | int(header[8])<<7 | int(header[9])

	// Read all tag data
	tagData := make([]byte, size)
	if _, err := io.ReadFull(file, tagData); err != nil {
		return nil, err
	}

	// Remove footer if present (10 bytes, starts with "3DI")
	if footerPresent && len(tagData) >= 10 {
		footerStart := len(tagData) - 10
		if footerStart >= 0 && string(tagData[footerStart:footerStart+3]) == "3DI" {
			tagData = tagData[:footerStart]
		}
	}

	// Skip extended header if present
	if extendedHeader {
		if skip := extendedHeaderSize(tagData, majorVersion); skip > 0 && skip < len(tagData) {
			tagData = tagData[skip:]
		}
	}

	metadata := &AudioMetadata{}

	// Parse frames based on version
	if majorVersion == 2 {
		parseID3v22Frames(tagData, metadata, unsync)
	} else {
		// ID3v2.3 and ID3v2.4
		parseID3v23Frames(tagData, metadata, majorVersion, unsync)
	}

	return metadata, nil
}

// parseID3v22Frames parses ID3v2.2 frames (3-char frame IDs)
func parseID3v22Frames(data []byte, metadata *AudioMetadata, tagUnsync bool) {
	pos := 0
	for pos+6 < len(data) {
		frameID := string(data[pos : pos+3])
		if frameID[0] == 0 {
			break // Padding
		}

		frameSize := int(data[pos+3])<<16 | int(data[pos+4])<<8 | int(data[pos+5])
		if frameSize <= 0 || pos+6+frameSize > len(data) {
			break
		}

		frameData := data[pos+6 : pos+6+frameSize]
		if tagUnsync {
			frameData = removeUnsync(frameData)
		}
		value := firstTextValue(extractTextFrame(frameData))

		switch frameID {
		case "TT2": // Title
			metadata.Title = value
		case "TP1": // Artist
			metadata.Artist = value
		case "TP2": // Album Artist
			metadata.AlbumArtist = value
		case "TAL": // Album
			metadata.Album = value
		case "TYE": // Year
			metadata.Year = value
		case "TCO": // Genre
			metadata.Genre = cleanGenre(value)
		case "TRK": // Track
			metadata.TrackNumber = parseTrackNumber(value)
		case "TPA": // Disc
			metadata.DiscNumber = parseTrackNumber(value)
		}

		pos += 6 + frameSize
	}
}

// parseID3v23Frames parses ID3v2.3 and ID3v2.4 frames (4-char frame IDs)
func parseID3v23Frames(data []byte, metadata *AudioMetadata, version byte, tagUnsync bool) {
	pos := 0
	for pos+10 < len(data) {
		frameID := string(data[pos : pos+4])
		if frameID[0] == 0 {
			break // Padding
		}

		var frameSize int
		if version == 4 {
			// ID3v2.4 uses syncsafe integers
			frameSize = int(data[pos+4])<<21 | int(data[pos+5])<<14 | int(data[pos+6])<<7 | int(data[pos+7])
		} else {
			// ID3v2.3 uses regular integers
			frameSize = int(data[pos+4])<<24 | int(data[pos+5])<<16 | int(data[pos+6])<<8 | int(data[pos+7])
		}

		if frameSize <= 0 || pos+10+frameSize > len(data) {
			break
		}

		frameData := data[pos+10 : pos+10+frameSize]

		statusFlags := data[pos+8]
		_ = statusFlags
		formatFlags := data[pos+9]

		// Handle frame-specific flags
		if version == 3 {
			// ID3v2.3 format flags: compression/encryption/grouping not supported
			const (
				id3v23FlagCompression = 0x80
				id3v23FlagEncryption  = 0x40
				id3v23FlagGrouping    = 0x20
			)
			if formatFlags&(id3v23FlagCompression|id3v23FlagEncryption) != 0 {
				pos += 10 + frameSize
				continue
			}
			if formatFlags&id3v23FlagGrouping != 0 {
				if len(frameData) < 1 {
					pos += 10 + frameSize
					continue
				}
				frameData = frameData[1:] // skip group ID
			}
			if tagUnsync {
				frameData = removeUnsync(frameData)
			}
		} else if version == 4 {
			// ID3v2.4 format flags: grouping, compression, encryption, unsync, data length indicator
			const (
				id3v24FlagGrouping      = 0x40
				id3v24FlagCompression   = 0x08
				id3v24FlagEncryption    = 0x04
				id3v24FlagUnsync        = 0x02
				id3v24FlagDataLen       = 0x01
			)
			if formatFlags&id3v24FlagGrouping != 0 {
				if len(frameData) < 1 {
					pos += 10 + frameSize
					continue
				}
				frameData = frameData[1:] // skip group ID
			}
			if formatFlags&id3v24FlagDataLen != 0 {
				if len(frameData) < 4 {
					pos += 10 + frameSize
					continue
				}
				frameData = frameData[4:]
			}
			if formatFlags&id3v24FlagUnsync != 0 || tagUnsync {
				frameData = removeUnsync(frameData)
			}
			if formatFlags&(id3v24FlagCompression|id3v24FlagEncryption) != 0 {
				pos += 10 + frameSize
				continue
			}
		}

		value := firstTextValue(extractTextFrame(frameData))

		switch frameID {
		case "TIT2": // Title
			metadata.Title = value
		case "TPE1": // Artist
			metadata.Artist = value
		case "TPE2": // Album Artist
			metadata.AlbumArtist = value
		case "TALB": // Album
			metadata.Album = value
		case "TYER", "TDRC": // Year
			metadata.Year = value
			if len(value) >= 4 {
				metadata.Date = value
			}
		case "TCON": // Genre
			metadata.Genre = cleanGenre(value)
		case "TRCK": // Track
			metadata.TrackNumber = parseTrackNumber(value)
		case "TPOS": // Disc
			metadata.DiscNumber = parseTrackNumber(value)
		case "TSRC": // ISRC
			metadata.ISRC = value
		}

		pos += 10 + frameSize
	}
}

// readID3v1 reads ID3v1 tag from end of file
func readID3v1(file *os.File) (*AudioMetadata, error) {
	// Seek to last 128 bytes
	if _, err := file.Seek(-128, io.SeekEnd); err != nil {
		return nil, err
	}

	tag := make([]byte, 128)
	if _, err := io.ReadFull(file, tag); err != nil {
		return nil, err
	}

	// Check for "TAG" identifier
	if string(tag[0:3]) != "TAG" {
		return nil, fmt.Errorf("no ID3v1 tag")
	}

	metadata := &AudioMetadata{
		Title:  strings.TrimRight(string(tag[3:33]), " \x00"),
		Artist: strings.TrimRight(string(tag[33:63]), " \x00"),
		Album:  strings.TrimRight(string(tag[63:93]), " \x00"),
		Year:   strings.TrimRight(string(tag[93:97]), " \x00"),
	}

	// ID3v1.1 track number (if byte 125 is 0 and byte 126 is not)
	if tag[125] == 0 && tag[126] != 0 {
		metadata.TrackNumber = int(tag[126])
	}

	// Genre index
	genreIndex := int(tag[127])
	if genreIndex < len(id3v1Genres) {
		metadata.Genre = id3v1Genres[genreIndex]
	}

	return metadata, nil
}

// extractTextFrame extracts text from ID3 text frame
func extractTextFrame(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	encoding := data[0]
	text := data[1:]

	switch encoding {
	case 0: // ISO-8859-1
		return strings.TrimRight(string(text), "\x00")
	case 1: // UTF-16 with BOM
		return decodeUTF16(text)
	case 2: // UTF-16BE
		return decodeUTF16BE(text)
	case 3: // UTF-8
		return strings.TrimRight(string(text), "\x00")
	default:
		return strings.TrimRight(string(text), "\x00")
	}
}

// decodeUTF16 decodes UTF-16 with BOM
func decodeUTF16(data []byte) string {
	if len(data) < 2 {
		return ""
	}

	// Check BOM
	var littleEndian bool
	if data[0] == 0xFF && data[1] == 0xFE {
		littleEndian = true
		data = data[2:]
	} else if data[0] == 0xFE && data[1] == 0xFF {
		littleEndian = false
		data = data[2:]
	}

	return decodeUTF16Data(data, littleEndian)
}

// decodeUTF16BE decodes UTF-16 Big Endian
func decodeUTF16BE(data []byte) string {
	return decodeUTF16Data(data, false)
}

// decodeUTF16Data decodes UTF-16 data
func decodeUTF16Data(data []byte, littleEndian bool) string {
	if len(data) < 2 {
		return ""
	}

	var runes []rune
	for i := 0; i+1 < len(data); i += 2 {
		var r uint16
		if littleEndian {
			r = uint16(data[i]) | uint16(data[i+1])<<8
		} else {
			r = uint16(data[i])<<8 | uint16(data[i+1])
		}
		if r == 0 {
			break
		}
		runes = append(runes, rune(r))
	}
	return string(runes)
}

// cleanGenre removes ID3 genre number format like "(17)" or "(17)Rock"
func cleanGenre(genre string) string {
	if len(genre) == 0 {
		return ""
	}

	// Handle "(17)" or "(17)Rock" format
	if genre[0] == '(' {
		end := strings.Index(genre, ")")
		if end > 0 {
			numStr := genre[1:end]
			if num, err := strconv.Atoi(numStr); err == nil && num < len(id3v1Genres) {
				// If there's text after the number, use it
				if end+1 < len(genre) {
					return genre[end+1:]
				}
				return id3v1Genres[num]
			}
		}
	}
	return genre
}

// parseTrackNumber extracts track number from "1/10" or "1" format
func parseTrackNumber(s string) int {
	s = strings.TrimSpace(s)
	if idx := strings.Index(s, "/"); idx > 0 {
		s = s[:idx]
	}
	num, _ := strconv.Atoi(s)
	return num
}

// removeUnsync removes ID3 unsynchronization (0xFF 0x00 -> 0xFF)
func removeUnsync(data []byte) []byte {
	if len(data) == 0 {
		return data
	}
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); i++ {
		b := data[i]
		out = append(out, b)
		if b == 0xFF && i+1 < len(data) && data[i+1] == 0x00 {
			i++
		}
	}
	return out
}

// extendedHeaderSize returns the total number of bytes to skip for the extended header
func extendedHeaderSize(data []byte, version byte) int {
	if len(data) < 4 {
		return 0
	}
	var size int
	if version == 3 {
		size = int(binary.BigEndian.Uint32(data[:4]))
	} else if version == 4 {
		size = syncsafeToInt(data[:4])
	} else {
		return 0
	}
	if size <= 0 {
		return 0
	}
	total := size + 4
	if total <= len(data) {
		return total
	}
	if size <= len(data) {
		return size
	}
	return 0
}

// syncsafeToInt decodes a 4-byte syncsafe integer
func syncsafeToInt(b []byte) int {
	if len(b) < 4 {
		return 0
	}
	return int(b[0])<<21 | int(b[1])<<14 | int(b[2])<<7 | int(b[3])
}

// firstTextValue returns the first value in a null-separated text list
func firstTextValue(s string) string {
	if idx := strings.IndexByte(s, 0); idx >= 0 {
		return s[:idx]
	}
	return s
}

// GetMP3Quality reads MP3 audio quality info
func GetMP3Quality(filePath string) (*MP3Quality, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	quality := &MP3Quality{}

	// Get file size for duration estimation
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}
	fileSize := stat.Size()

	// Skip ID3v2 header if present
	header := make([]byte, 10)
	if _, err := io.ReadFull(file, header); err != nil {
		return nil, err
	}

	var audioStart int64 = 0
	if string(header[0:3]) == "ID3" {
		tagSize := int64(header[6])<<21 | int64(header[7])<<14 | int64(header[8])<<7 | int64(header[9])
		audioStart = 10 + tagSize
	}

	// Seek to audio start
	file.Seek(audioStart, io.SeekStart)

	// Find first valid MP3 frame
	frameHeader := make([]byte, 4)
	for i := 0; i < 10000; i++ { // Search first 10KB
		if _, err := io.ReadFull(file, frameHeader); err != nil {
			break
		}

		// Check for sync word (11 set bits)
		if frameHeader[0] == 0xFF && (frameHeader[1]&0xE0) == 0xE0 {
			// Parse frame header
			version := (frameHeader[1] >> 3) & 0x03
			layer := (frameHeader[1] >> 1) & 0x03
			bitrateIdx := (frameHeader[2] >> 4) & 0x0F
			sampleRateIdx := (frameHeader[2] >> 2) & 0x03

			// Get sample rate
			sampleRates := [][]int{
				{11025, 12000, 8000},  // MPEG 2.5
				{0, 0, 0},             // Reserved
				{22050, 24000, 16000}, // MPEG 2
				{44100, 48000, 32000}, // MPEG 1
			}
			if version < 4 && sampleRateIdx < 3 {
				quality.SampleRate = sampleRates[version][sampleRateIdx]
			}

			// Get bitrate (for MPEG 1 Layer 3)
			if version == 3 && layer == 1 { // MPEG 1, Layer 3
				bitrates := []int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}
				if bitrateIdx < 16 {
					quality.Bitrate = bitrates[bitrateIdx] * 1000
				}
			}

			// MP3 is always 16-bit PCM when decoded
			quality.BitDepth = 16

			// Estimate duration from file size and bitrate
			if quality.Bitrate > 0 {
				audioSize := fileSize - audioStart - 128 // Subtract ID3v1 tag
				if audioSize > 0 {
					quality.Duration = int(audioSize * 8 / int64(quality.Bitrate))
				}
			}

			break
		}

		// Seek back 3 bytes to continue search
		file.Seek(-3, io.SeekCurrent)
	}

	return quality, nil
}

// =============================================================================
// Ogg/Opus Vorbis Comment Reading
// =============================================================================

// ReadOggVorbisComments reads Vorbis comments from Ogg/Opus files
func ReadOggVorbisComments(filePath string) (*AudioMetadata, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	metadata := &AudioMetadata{}

	packets, err := collectOggPackets(file, 30, 80)
	if err != nil && len(packets) == 0 {
		return nil, err
	}

	streamType := detectOggStreamType(packets)
	for _, pkt := range packets {
		if streamType == oggStreamOpus {
			if len(pkt) > 8 && string(pkt[0:8]) == "OpusTags" {
				parseVorbisComments(pkt[8:], metadata)
				break
			}
			continue
		}
		if streamType == oggStreamVorbis || streamType == oggStreamUnknown {
			if len(pkt) > 7 && pkt[0] == 0x03 && string(pkt[1:7]) == "vorbis" {
				parseVorbisComments(pkt[7:], metadata)
				break
			}
		}
		// Fallback: if unknown, still try OpusTags
		if streamType == oggStreamUnknown {
			if len(pkt) > 8 && string(pkt[0:8]) == "OpusTags" {
				parseVorbisComments(pkt[8:], metadata)
				break
			}
		}
	}

	if metadata.Title == "" && metadata.Artist == "" {
		return nil, fmt.Errorf("no Vorbis comments found")
	}

	return metadata, nil
}

type oggPage struct {
	headerType   byte
	segmentTable []byte
	data         []byte
}

// readOggPageWithHeader reads a single Ogg page including header info
func readOggPageWithHeader(file *os.File) (*oggPage, error) {
	// Read page header
	header := make([]byte, 27)
	if _, err := io.ReadFull(file, header); err != nil {
		return nil, err
	}

	// Check capture pattern "OggS"
	if string(header[0:4]) != "OggS" {
		return nil, fmt.Errorf("not an Ogg page")
	}

	headerType := header[5]
	numSegments := int(header[26])

	// Read segment table
	segmentTable := make([]byte, numSegments)
	if _, err := io.ReadFull(file, segmentTable); err != nil {
		return nil, err
	}

	// Calculate total page size
	var pageSize int
	for _, seg := range segmentTable {
		pageSize += int(seg)
	}

	// Read page data
	pageData := make([]byte, pageSize)
	if _, err := io.ReadFull(file, pageData); err != nil {
		return nil, err
	}

	return &oggPage{
		headerType:   headerType,
		segmentTable: segmentTable,
		data:         pageData,
	}, nil
}

// readOggPage reads a single Ogg page (data only)
func readOggPage(file *os.File) ([]byte, error) {
	page, err := readOggPageWithHeader(file)
	if err != nil {
		return nil, err
	}
	return page.data, nil
}

// collectOggPackets reads Ogg pages and returns reassembled packets
func collectOggPackets(file *os.File, maxPackets, maxPages int) ([][]byte, error) {
	const maxPacketSize = 10 * 1024 * 1024
	var packets [][]byte
	var cur []byte
	skipPacket := false

	for pageNum := 0; pageNum < maxPages && len(packets) < maxPackets; pageNum++ {
		page, err := readOggPageWithHeader(file)
		if err != nil {
			if len(packets) > 0 {
				return packets, nil
			}
			return nil, err
		}

		// If this page is not a continuation but we have partial packet, drop it
		if page.headerType&0x01 == 0 && len(cur) > 0 {
			cur = nil
			skipPacket = false
		}

		offset := 0
		for _, seg := range page.segmentTable {
			segLen := int(seg)
			if offset+segLen > len(page.data) {
				return packets, fmt.Errorf("invalid ogg segment size")
			}

			if skipPacket {
				offset += segLen
				if segLen < 255 {
					skipPacket = false
				}
				continue
			}

			if len(cur)+segLen > maxPacketSize {
				// Skip this oversized packet
				cur = nil
				skipPacket = true
				offset += segLen
				if segLen < 255 {
					skipPacket = false
				}
				continue
			}

			cur = append(cur, page.data[offset:offset+segLen]...)
			offset += segLen

			if segLen < 255 {
				if len(cur) > 0 {
					packets = append(packets, cur)
				}
				cur = nil
				if len(packets) >= maxPackets {
					return packets, nil
				}
			}
		}
	}

	return packets, nil
}

type oggStreamType int

const (
	oggStreamUnknown oggStreamType = iota
	oggStreamOpus
	oggStreamVorbis
)

func detectOggStreamType(packets [][]byte) oggStreamType {
	for _, p := range packets {
		if len(p) >= 8 && string(p[0:8]) == "OpusHead" {
			return oggStreamOpus
		}
		if len(p) > 7 && p[0] == 0x01 && string(p[1:7]) == "vorbis" {
			return oggStreamVorbis
		}
	}
	return oggStreamUnknown
}

// parseVorbisComments parses Vorbis comment block
func parseVorbisComments(data []byte, metadata *AudioMetadata) {
	if len(data) < 4 {
		return
	}

	reader := bytes.NewReader(data)

	// Read vendor string length
	var vendorLen uint32
	if err := binary.Read(reader, binary.LittleEndian, &vendorLen); err != nil {
		return
	}

	// Skip vendor string
	if vendorLen > uint32(len(data)-4) {
		return
	}
	vendor := make([]byte, vendorLen)
	if _, err := reader.Read(vendor); err != nil {
		return
	}

	// Read comment count
	var commentCount uint32
	if err := binary.Read(reader, binary.LittleEndian, &commentCount); err != nil {
		return
	}

	// Read each comment
	for i := uint32(0); i < commentCount && i < 100; i++ {
		var commentLen uint32
		if err := binary.Read(reader, binary.LittleEndian, &commentLen); err != nil {
			break
		}

		if commentLen > 10000 { // Sanity check
			break
		}

		comment := make([]byte, commentLen)
		if _, err := reader.Read(comment); err != nil {
			break
		}

		// Parse "KEY=VALUE" format
		parts := strings.SplitN(string(comment), "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.ToUpper(parts[0])
		value := parts[1]

		switch key {
		case "TITLE":
			metadata.Title = value
		case "ARTIST":
			metadata.Artist = value
		case "ALBUMARTIST", "ALBUM_ARTIST", "ALBUM ARTIST":
			metadata.AlbumArtist = value
		case "ALBUM":
			metadata.Album = value
		case "DATE", "YEAR":
			metadata.Date = value
			if len(value) >= 4 {
				metadata.Year = value[:4]
			}
		case "GENRE":
			metadata.Genre = value
		case "TRACKNUMBER", "TRACK":
			metadata.TrackNumber = parseTrackNumber(value)
		case "DISCNUMBER", "DISC":
			metadata.DiscNumber = parseTrackNumber(value)
		case "ISRC":
			metadata.ISRC = value
		}
	}
}

// GetOggQuality reads Ogg/Opus audio quality info
func GetOggQuality(filePath string) (*OggQuality, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	quality := &OggQuality{}
	isOpus := false

	packets, err := collectOggPackets(file, 5, 10)
	if err != nil && len(packets) == 0 {
		return nil, err
	}

	streamType := detectOggStreamType(packets)
	if streamType == oggStreamUnknown {
		// Fallback to file extension
		if strings.HasSuffix(strings.ToLower(filePath), ".opus") {
			streamType = oggStreamOpus
		} else {
			streamType = oggStreamVorbis
		}
	}

	if streamType == oggStreamOpus {
		isOpus = true
		for _, pkt := range packets {
			if len(pkt) >= 19 && string(pkt[0:8]) == "OpusHead" {
				quality.SampleRate = int(binary.LittleEndian.Uint32(pkt[12:16]))
				if quality.SampleRate == 0 {
					quality.SampleRate = 48000
				}
				quality.BitDepth = 16
				break
			}
		}
	} else {
		for _, pkt := range packets {
			if len(pkt) > 29 && pkt[0] == 0x01 && string(pkt[1:7]) == "vorbis" {
				quality.SampleRate = int(binary.LittleEndian.Uint32(pkt[12:16]))
				quality.BitDepth = 16
				break
			}
		}
	}

	// Get file size for duration estimation
	stat, err := file.Stat()
	if err == nil {
		// Very rough duration estimate based on file size
		// Assume ~128kbps average for Opus, ~160kbps for Vorbis
		avgBitrate := 128000
		if !isOpus {
			avgBitrate = 160000
		}
		quality.Duration = int(stat.Size() * 8 / int64(avgBitrate))
	}

	return quality, nil
}

// =============================================================================
// ID3v1 Genre List
// =============================================================================

var id3v1Genres = []string{
	"Blues", "Classic Rock", "Country", "Dance", "Disco", "Funk", "Grunge",
	"Hip-Hop", "Jazz", "Metal", "New Age", "Oldies", "Other", "Pop", "R&B",
	"Rap", "Reggae", "Rock", "Techno", "Industrial", "Alternative", "Ska",
	"Death Metal", "Pranks", "Soundtrack", "Euro-Techno", "Ambient",
	"Trip-Hop", "Vocal", "Jazz+Funk", "Fusion", "Trance", "Classical",
	"Instrumental", "Acid", "House", "Game", "Sound Clip", "Gospel",
	"Noise", "AlternRock", "Bass", "Soul", "Punk", "Space", "Meditative",
	"Instrumental Pop", "Instrumental Rock", "Ethnic", "Gothic",
	"Darkwave", "Techno-Industrial", "Electronic", "Pop-Folk", "Eurodance",
	"Dream", "Southern Rock", "Comedy", "Cult", "Gangsta", "Top 40",
	"Christian Rap", "Pop/Funk", "Jungle", "Native American", "Cabaret",
	"New Wave", "Psychedelic", "Rave", "Showtunes", "Trailer", "Lo-Fi",
	"Tribal", "Acid Punk", "Acid Jazz", "Polka", "Retro", "Musical",
	"Rock & Roll", "Hard Rock", "Folk", "Folk-Rock", "National Folk",
	"Swing", "Fast Fusion", "Bebop", "Latin", "Revival", "Celtic",
	"Bluegrass", "Avantgarde", "Gothic Rock", "Progressive Rock",
	"Psychedelic Rock", "Symphonic Rock", "Slow Rock", "Big Band",
	"Chorus", "Easy Listening", "Acoustic", "Humour", "Speech", "Chanson",
	"Opera", "Chamber Music", "Sonata", "Symphony", "Booty Bass", "Primus",
	"Porn Groove", "Satire", "Slow Jam", "Club", "Tango", "Samba",
	"Folklore", "Ballad", "Power Ballad", "Rhythmic Soul", "Freestyle",
	"Duet", "Punk Rock", "Drum Solo", "A capella", "Euro-House",
	"Dance Hall", "Goa", "Drum & Bass", "Club-House", "Hardcore",
	"Terror", "Indie", "BritPop", "Negerpunk", "Polsk Punk", "Beat",
	"Christian Gangsta Rap", "Heavy Metal", "Black Metal", "Crossover",
	"Contemporary Christian", "Christian Rock", "Merengue", "Salsa",
	"Thrash Metal", "Anime", "J-Pop", "Synthpop",
}

// =============================================================================
// Cover Art Extraction
// =============================================================================

// extractMP3CoverArt extracts cover art from MP3 file (APIC frame)
func extractMP3CoverArt(filePath string) ([]byte, string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()

	// Read ID3v2 header
	header := make([]byte, 10)
	if _, err := io.ReadFull(file, header); err != nil {
		return nil, "", err
	}

	if string(header[0:3]) != "ID3" {
		return nil, "", fmt.Errorf("no ID3v2 header")
	}

	majorVersion := header[3]
	size := int(header[6])<<21 | int(header[7])<<14 | int(header[8])<<7 | int(header[9])

	tagData := make([]byte, size)
	if _, err := io.ReadFull(file, tagData); err != nil {
		return nil, "", err
	}

	// Parse frames looking for APIC (Attached Picture)
	pos := 0
	var frameIDLen, headerLen int
	if majorVersion == 2 {
		frameIDLen = 3
		headerLen = 6
	} else {
		frameIDLen = 4
		headerLen = 10
	}

	for pos+headerLen < len(tagData) {
		frameID := string(tagData[pos : pos+frameIDLen])
		if frameID[0] == 0 {
			break
		}

		var frameSize int
		if majorVersion == 2 {
			frameSize = int(tagData[pos+3])<<16 | int(tagData[pos+4])<<8 | int(tagData[pos+5])
		} else if majorVersion == 4 {
			frameSize = int(tagData[pos+4])<<21 | int(tagData[pos+5])<<14 | int(tagData[pos+6])<<7 | int(tagData[pos+7])
		} else {
			frameSize = int(tagData[pos+4])<<24 | int(tagData[pos+5])<<16 | int(tagData[pos+6])<<8 | int(tagData[pos+7])
		}

		if frameSize <= 0 || pos+headerLen+frameSize > len(tagData) {
			break
		}

		// Check for APIC (ID3v2.3/2.4) or PIC (ID3v2.2)
		if (frameIDLen == 4 && frameID == "APIC") || (frameIDLen == 3 && frameID == "PIC") {
			frameData := tagData[pos+headerLen : pos+headerLen+frameSize]
			imageData, mimeType := parseAPICFrame(frameData, majorVersion)
			if len(imageData) > 0 {
				return imageData, mimeType, nil
			}
		}

		pos += headerLen + frameSize
	}

	return nil, "", fmt.Errorf("no cover art found")
}

// parseAPICFrame parses APIC frame data
func parseAPICFrame(data []byte, version byte) ([]byte, string) {
	if len(data) < 4 {
		return nil, ""
	}

	pos := 0
	encoding := data[pos]
	pos++

	// Read MIME type
	var mimeType string
	if version == 2 {
		// ID3v2.2: 3-byte image format (JPG, PNG)
		if pos+3 > len(data) {
			return nil, ""
		}
		format := string(data[pos : pos+3])
		pos += 3
		switch format {
		case "JPG":
			mimeType = "image/jpeg"
		case "PNG":
			mimeType = "image/png"
		default:
			mimeType = "image/jpeg"
		}
	} else {
		// ID3v2.3/2.4: null-terminated MIME string
		end := pos
		for end < len(data) && data[end] != 0 {
			end++
		}
		mimeType = string(data[pos:end])
		pos = end + 1
	}

	if pos >= len(data) {
		return nil, ""
	}

	// Skip picture type
	// pictureType := data[pos]
	pos++

	// Skip description (null-terminated, may be UTF-16)
	if encoding == 0 || encoding == 3 {
		// ISO-8859-1 or UTF-8
		for pos < len(data) && data[pos] != 0 {
			pos++
		}
		pos++ // Skip null
	} else {
		// UTF-16: look for double null
		for pos+1 < len(data) {
			if data[pos] == 0 && data[pos+1] == 0 {
				pos += 2
				break
			}
			pos++
		}
	}

	if pos >= len(data) {
		return nil, ""
	}

	// Rest is image data
	return data[pos:], mimeType
}

// extractOggCoverArt extracts cover art from Ogg/Opus file (METADATA_BLOCK_PICTURE)
func extractOggCoverArt(filePath string) ([]byte, string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()

	packets, err := collectOggPackets(file, 30, 80)
	if err != nil && len(packets) == 0 {
		return nil, "", err
	}

	streamType := detectOggStreamType(packets)
	for _, pkt := range packets {
		var comments []byte
		if streamType == oggStreamOpus {
			if len(pkt) > 8 && string(pkt[0:8]) == "OpusTags" {
				comments = pkt[8:]
			}
		} else {
			if len(pkt) > 7 && pkt[0] == 0x03 && string(pkt[1:7]) == "vorbis" {
				comments = pkt[7:]
			}
		}
		if len(comments) == 0 && streamType == oggStreamUnknown {
			if len(pkt) > 8 && string(pkt[0:8]) == "OpusTags" {
				comments = pkt[8:]
			} else if len(pkt) > 7 && pkt[0] == 0x03 && string(pkt[1:7]) == "vorbis" {
				comments = pkt[7:]
			}
		}

		if len(comments) > 0 {
			imageData, mimeType := extractPictureFromVorbisComments(comments)
			if len(imageData) > 0 {
				return imageData, mimeType, nil
			}
		}
	}

	return nil, "", fmt.Errorf("no cover art found")
}

// extractPictureFromVorbisComments looks for METADATA_BLOCK_PICTURE in Vorbis comments
func extractPictureFromVorbisComments(data []byte) ([]byte, string) {
	if len(data) < 8 {
		return nil, ""
	}

	reader := bytes.NewReader(data)

	// Skip vendor string
	var vendorLen uint32
	if err := binary.Read(reader, binary.LittleEndian, &vendorLen); err != nil {
		return nil, ""
	}
	if vendorLen > uint32(len(data)-4) {
		return nil, ""
	}
	reader.Seek(int64(vendorLen), io.SeekCurrent)

	// Read comment count
	var commentCount uint32
	if err := binary.Read(reader, binary.LittleEndian, &commentCount); err != nil {
		return nil, ""
	}

	// Look for METADATA_BLOCK_PICTURE
	for i := uint32(0); i < commentCount && i < 100; i++ {
		var commentLen uint32
		if err := binary.Read(reader, binary.LittleEndian, &commentLen); err != nil {
			break
		}
		if commentLen > 10000000 { // 10MB sanity check
			break
		}

		comment := make([]byte, commentLen)
		if _, err := reader.Read(comment); err != nil {
			break
		}

		// Check for METADATA_BLOCK_PICTURE=
		key := "METADATA_BLOCK_PICTURE="
		if len(comment) > len(key) && strings.ToUpper(string(comment[:len(key)])) == key {
			// Base64-encoded FLAC picture block
			b64Data := comment[len(key):]
			decoded := make([]byte, base64StdDecodeLen(len(b64Data)))
			n, err := base64StdDecode(decoded, b64Data)
			if err != nil {
				continue
			}
			decoded = decoded[:n]

			// Parse FLAC picture block
			imageData, mimeType := parseFLACPictureBlock(decoded)
			if len(imageData) > 0 {
				return imageData, mimeType
			}
		}
	}

	return nil, ""
}

// parseFLACPictureBlock parses FLAC PICTURE metadata block format
func parseFLACPictureBlock(data []byte) ([]byte, string) {
	if len(data) < 32 {
		return nil, ""
	}

	reader := bytes.NewReader(data)

	// Picture type (4 bytes)
	var pictureType uint32
	binary.Read(reader, binary.BigEndian, &pictureType)

	// MIME type length (4 bytes)
	var mimeLen uint32
	binary.Read(reader, binary.BigEndian, &mimeLen)
	if mimeLen > 256 {
		return nil, ""
	}

	// MIME type
	mimeBytes := make([]byte, mimeLen)
	reader.Read(mimeBytes)
	mimeType := string(mimeBytes)

	// Description length (4 bytes)
	var descLen uint32
	binary.Read(reader, binary.BigEndian, &descLen)
	if descLen > 10000 {
		return nil, ""
	}

	// Skip description
	reader.Seek(int64(descLen), io.SeekCurrent)

	// Skip width, height, color depth, colors used (16 bytes)
	reader.Seek(16, io.SeekCurrent)

	// Image data length (4 bytes)
	var dataLen uint32
	binary.Read(reader, binary.BigEndian, &dataLen)
	if dataLen > 10000000 { // 10MB
		return nil, ""
	}

	// Image data
	imageData := make([]byte, dataLen)
	reader.Read(imageData)

	return imageData, mimeType
}

// base64StdDecodeLen returns decoded length
func base64StdDecodeLen(n int) int {
	return n * 6 / 8
}

// base64StdDecode decodes base64 data (simplified)
func base64StdDecode(dst, src []byte) (int, error) {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

	decodeMap := make([]byte, 256)
	for i := range decodeMap {
		decodeMap[i] = 0xFF
	}
	for i := 0; i < len(alphabet); i++ {
		decodeMap[alphabet[i]] = byte(i)
	}

	si, di := 0, 0
	for si < len(src) {
		// Skip whitespace and newlines
		for si < len(src) && (src[si] == '\n' || src[si] == '\r' || src[si] == ' ' || src[si] == '\t') {
			si++
		}
		if si >= len(src) {
			break
		}

		// Read 4 characters
		var vals [4]byte
		var valCount int
		for valCount < 4 && si < len(src) {
			c := src[si]
			si++
			if c == '=' {
				vals[valCount] = 0
				valCount++
			} else if c == '\n' || c == '\r' || c == ' ' || c == '\t' {
				continue
			} else if decodeMap[c] != 0xFF {
				vals[valCount] = decodeMap[c]
				valCount++
			}
		}

		if valCount < 2 {
			break
		}

		// Decode
		if di < len(dst) {
			dst[di] = vals[0]<<2 | vals[1]>>4
			di++
		}
		if valCount >= 3 && di < len(dst) {
			dst[di] = vals[1]<<4 | vals[2]>>2
			di++
		}
		if valCount >= 4 && di < len(dst) {
			dst[di] = vals[2]<<6 | vals[3]
			di++
		}
	}

	return di, nil
}

// extractAnyCoverArt extracts cover art from any supported audio file
func extractAnyCoverArt(filePath string) ([]byte, string, error) {
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".flac":
		// Use existing ExtractCoverArt function
		data, err := ExtractCoverArt(filePath)
		if err != nil {
			return nil, "", err
		}
		// Detect MIME type from magic bytes
		mimeType := "image/jpeg"
		if len(data) > 8 && string(data[1:4]) == "PNG" {
			mimeType = "image/png"
		}
		return data, mimeType, nil

	case ".mp3":
		return extractMP3CoverArt(filePath)

	case ".opus", ".ogg":
		return extractOggCoverArt(filePath)

	case ".m4a":
		// M4A cover extraction would need more complex MP4 atom parsing
		// For now, return error
		return nil, "", fmt.Errorf("M4A cover extraction not yet supported")

	default:
		return nil, "", fmt.Errorf("unsupported format: %s", ext)
	}
}

// SaveCoverToCache extracts and saves cover art to cache directory
// Returns the path to the saved cover image, or empty string if no cover found
func SaveCoverToCache(filePath, cacheDir string) (string, error) {
	// Generate cache filename from file path + size + mtime to reduce stale cache
	cacheKey := filePath
	if stat, err := os.Stat(filePath); err == nil {
		cacheKey = fmt.Sprintf("%s|%d|%d", filePath, stat.Size(), stat.ModTime().UnixNano())
	}
	hash := hashString(cacheKey)

	// Check if cover already cached
	jpgPath := filepath.Join(cacheDir, fmt.Sprintf("cover_%x.jpg", hash))
	pngPath := filepath.Join(cacheDir, fmt.Sprintf("cover_%x.png", hash))

	if _, err := os.Stat(jpgPath); err == nil {
		return jpgPath, nil
	}
	if _, err := os.Stat(pngPath); err == nil {
		return pngPath, nil
	}

	// Extract cover art
	imageData, mimeType, err := extractAnyCoverArt(filePath)
	if err != nil {
		return "", err
	}

	// Ensure cache directory exists
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create cache dir: %w", err)
	}

	// Determine file extension
	var cachePath string
	if strings.Contains(mimeType, "png") {
		cachePath = pngPath
	} else {
		cachePath = jpgPath
	}

	// Write to file
	if err := os.WriteFile(cachePath, imageData, 0644); err != nil {
		return "", fmt.Errorf("failed to write cover: %w", err)
	}

	return cachePath, nil
}
