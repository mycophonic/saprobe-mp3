package maindata

import (
	"errors"
	"fmt"
	"io"

	"github.com/mycophonic/saprobe-mp3/internal/bits"
	"github.com/mycophonic/saprobe-mp3/internal/consts"
	"github.com/mycophonic/saprobe-mp3/internal/frameheader"
	"github.com/mycophonic/saprobe-mp3/internal/sideinfo"
)

const (
	// SCFSI group boundaries (ISO 11172-3 §2.4.2.7). The long-block scale
	// factor bands are partitioned into four groups:
	//   Group 0: bands [0, 6)    Group 1: bands [6, 11)
	//   Group 2: bands [11, 16)  Group 3: bands [16, 21)
	scfsiGroup1 = 6
	scfsiGroup2 = 11
	scfsiGroup3 = 16

	mpeg2LongSfbCount  = 22 // MPEG2 long-block scale factor count
	mpeg2ShortSfbCount = 13 // MPEG2 short-block scale factor band count
)

// MainData contains the decoded MPEG Layer 3 main data including scale factors and frequency lines.
type MainData struct {
	ScalefacL [2][2][22]int      // 0-4 bits
	ScalefacS [2][2][13][3]int   // 0-4 bits
	Is        [2][2][576]float32 // Huffman coded freq. lines
	Count1    [2][2]int          // set by Huffman decoder, not parsed from bitstream
}

// Reservoir manages the bit reservoir for main data across frames.
// The MP3 format allows frames to reference main data bytes from previous frames
// via the MainDataBegin pointer in side information.
//
// Max buffer: 511 (MPEG1 MainDataBegin) + 1405 (max main data) = 1916 bytes normal.
// 4096 provides margin for the insufficient-reservoir append path.
type Reservoir struct {
	buf [4096]byte
	len int
}

// Reset clears the reservoir, discarding all buffered main data.
// Called after a sync skip to prevent decoding from stale bytes.
func (r *Reservoir) Reset() {
	r.len = 0
}

var scalefacSizesMpeg1 = [16][2]int{ //nolint:gochecknoglobals // immutable lookup table
	{0, 0},
	{0, 1},
	{0, 2},
	{0, 3},
	{3, 0},
	{1, 1},
	{1, 2},
	{1, 3},
	{2, 1},
	{2, 2},
	{2, 3},
	{3, 1},
	{3, 2},
	{3, 3},
	{4, 2},
	{4, 3},
}

var scalefacSizesMpeg2 = [3][6][4]int{ //nolint:gochecknoglobals // immutable lookup table
	{
		{6, 5, 5, 5},
		{6, 5, 7, 3},
		{11, 10, 0, 0},
		{7, 7, 7, 0},
		{6, 6, 6, 3},
		{8, 8, 5, 0},
	},
	{
		{9, 9, 9, 9},
		{9, 9, 12, 6},
		{18, 18, 0, 0},
		{12, 12, 12, 0},
		{12, 9, 9, 6},
		{15, 12, 9, 0},
	},
	{
		{6, 9, 9, 9},
		{6, 9, 12, 6},
		{15, 18, 0, 0},
		{6, 15, 12, 0},
		{6, 12, 9, 6},
		{6, 18, 9, 0},
	},
}

var nSlen2 = initSlen() //nolint:gochecknoglobals // immutable lookup table (MPEG 2.0 slen for 'normal' mode)

//nolint:gocognit // the nested loops are the spec's scalefac_compress table, written out
func initSlen() (nSlen2 [512]int) {
	for i := range 4 {
		for j := range 3 {
			n := j + i*3
			nSlen2[n+500] = i | (j << 3) | (2 << 12) | (1 << 15) //nolint:mnd // slen packing: fields at bits 0, 3, 6, 9, 12 (table) and 15 (preflag)
		}
	}

	for i := range 5 {
		for j := range 5 {
			for k := range 4 {
				for l := range 4 {
					n := l + k*4 + j*16 + i*80                                 //nolint:mnd // row-major index into the 5×5×4×4 table
					nSlen2[n] = i | (j << 3) | (k << 6) | (l << 9) | (0 << 12) //nolint:mnd // slen packing: fields at bits 0, 3, 6, 9, 12 (table) and 15 (preflag)
				}
			}
		}
	}

	for i := range 5 {
		for j := range 5 {
			for k := range 4 {
				n := k + j*4 + i*20
				nSlen2[n+400] = i | (j << 3) | (k << 6) | (1 << 12) //nolint:mnd // slen packing: fields at bits 0, 3, 6, 9, 12 (table) and 15 (preflag)
			}
		}
	}

	return nSlen2
}

// Read reads and parses the main data from an MP3 frame into dst.
// The caller provides dst (typically embedded in Frame as a value field)
// and res (the persistent bit reservoir, also embedded in Frame).
//
// Returns (true, nil) on success. When the reservoir has insufficient data
// for MainDataBegin, the frame's main data is consumed into the reservoir
// for subsequent frames but decoding is skipped — returns (false, nil).
// The caller should output silence for that frame.
func Read(
	source consts.FullReader,
	header *frameheader.Header,
	sideInfo *sideinfo.SideInfo,
	dst *MainData,
	res *Reservoir,
) (bool, error) {
	nch := header.Channels()

	// Main data size is the frame minus header (4 bytes), side info, and optional CRC (2 bytes).
	mainDataSize := header.FrameSize() - header.SideInfoSize() - 4
	if header.Protected() {
		mainDataSize -= 2
	}

	offset := sideInfo.MainDataBegin

	// Build the bitstream from reservoir tail + current frame data.
	if offset > res.len {
		// Insufficient reservoir — consume the current frame's main data into the
		// reservoir for subsequent frames, but signal the caller to output silence.
		// Following mpg123's approach: accumulate data (don't discard existing
		// reservoir content) so subsequent frames can reference it.
		newLen := res.len + mainDataSize
		if newLen > len(res.buf) {
			//nolint:err113 // runtime value in error message required for debugging
			return false, fmt.Errorf(
				"mp3: maindata reservoir overflow: %d + %d > %d",
				res.len,
				mainDataSize,
				len(res.buf),
			)
		}

		count, err := source.ReadFull(res.buf[res.len:newLen])
		if count < mainDataSize {
			if errors.Is(err, io.EOF) {
				return false, &consts.UnexpectedEOFError{At: "maindata.Read (1)"}
			}

			return false, fmt.Errorf("mp3: maindata.Read (1): %w", err)
		}

		res.len = newLen

		return false, nil
	}

	// Normal path: shift reservoir tail to start of buffer, then read current data.
	if offset+mainDataSize > len(res.buf) {
		//nolint:err113 // runtime value in error message required for debugging
		return false, fmt.Errorf(
			"mp3: maindata reservoir overflow: %d + %d > %d",
			offset,
			mainDataSize,
			len(res.buf),
		)
	}

	tailStart := res.len - offset
	copy(res.buf[:offset], res.buf[tailStart:res.len])

	count, err := source.ReadFull(res.buf[offset : offset+mainDataSize])
	if count < mainDataSize {
		if errors.Is(err, io.EOF) {
			return false, &consts.UnexpectedEOFError{At: "maindata.Read (2)"}
		}

		return false, fmt.Errorf("mp3: maindata.Read (2): %w", err)
	}

	res.len = offset + mainDataSize

	// Stack-local bit reader over the reservoir buffer — zero heap allocation.
	var reader bits.Reader
	reader.Reset(res.buf[:res.len])

	if header.LowSamplingFrequency() == 1 {
		return true, getScaleFactorsMpeg2(&reader, header, sideInfo, dst)
	}

	return true, getScaleFactorsMpeg1(nch, &reader, header, sideInfo, dst)
}

//nolint:gocognit // scfsi, block type and mixed mode select the spec's scale-factor layout; hot path
func getScaleFactorsMpeg1(
	nch int,
	bitStream *bits.Reader,
	header *frameheader.Header,
	sideInfo *sideinfo.SideInfo,
	dst *MainData,
) error {
	for granule := range 2 {
		for channel := range nch {
			part2Start := bitStream.BitPos()
			// Number of bits in the bitstream for the bands
			slen1 := scalefacSizesMpeg1[sideInfo.ScalefacCompress[granule][channel]][0]

			slen2 := scalefacSizesMpeg1[sideInfo.ScalefacCompress[granule][channel]][1]
			if sideInfo.WinSwitchFlag[granule][channel] == 1 && sideInfo.BlockType[granule][channel] == 2 {
				if sideInfo.MixedBlockFlag[granule][channel] != 0 {
					for sfb := range 8 {
						dst.ScalefacL[granule][channel][sfb] = bitStream.ReadBits(slen1)
					}

					for sfb := 3; sfb < consts.SfbShortBands; sfb++ {
						// slen1 for band 3-5, slen2 for 6-11
						nbits := slen2
						if sfb < scfsiGroup1 {
							nbits = slen1
						}

						for win := range 3 {
							dst.ScalefacS[granule][channel][sfb][win] = bitStream.ReadBits(nbits)
						}
					}
				} else {
					for sfb := range consts.SfbShortBands {
						// slen1 for band 3-5, slen2 for 6-11
						nbits := slen2
						if sfb < scfsiGroup1 {
							nbits = slen1
						}

						for win := range 3 {
							dst.ScalefacS[granule][channel][sfb][win] = bitStream.ReadBits(nbits)
						}
					}
				}
			} else {
				// Scale factor bands 0-5
				if sideInfo.Scfsi[channel][0] == 0 || granule == 0 {
					for sfb := range scfsiGroup1 {
						dst.ScalefacL[granule][channel][sfb] = bitStream.ReadBits(slen1)
					}
				} else if sideInfo.Scfsi[channel][0] == 1 && granule == 1 {
					// Copy scalefactors from granule 0 to granule 1 (scfsi reuse, ISO 11172-3 §2.4.2.7).
					for sfb := range scfsiGroup1 {
						dst.ScalefacL[1][channel][sfb] = dst.ScalefacL[0][channel][sfb]
					}
				}
				// Scale factor bands 6-10
				if sideInfo.Scfsi[channel][1] == 0 || granule == 0 {
					for sfb := scfsiGroup1; sfb < scfsiGroup2; sfb++ {
						dst.ScalefacL[granule][channel][sfb] = bitStream.ReadBits(slen1)
					}
				} else if sideInfo.Scfsi[channel][1] == 1 && granule == 1 {
					// Copy scalefactors from granule 0 to granule 1
					for sfb := scfsiGroup1; sfb < scfsiGroup2; sfb++ {
						dst.ScalefacL[1][channel][sfb] = dst.ScalefacL[0][channel][sfb]
					}
				}
				// Scale factor bands 11-15
				if sideInfo.Scfsi[channel][2] == 0 || granule == 0 {
					for sfb := scfsiGroup2; sfb < scfsiGroup3; sfb++ {
						dst.ScalefacL[granule][channel][sfb] = bitStream.ReadBits(slen2)
					}
				} else if sideInfo.Scfsi[channel][2] == 1 && granule == 1 {
					// Copy scalefactors from granule 0 to granule 1
					for sfb := scfsiGroup2; sfb < scfsiGroup3; sfb++ {
						dst.ScalefacL[1][channel][sfb] = dst.ScalefacL[0][channel][sfb]
					}
				}
				// Scale factor bands 16-20
				if sideInfo.Scfsi[channel][3] == 0 || granule == 0 {
					for sfb := scfsiGroup3; sfb < consts.SfbLongBands; sfb++ {
						dst.ScalefacL[granule][channel][sfb] = bitStream.ReadBits(slen2)
					}
				} else if sideInfo.Scfsi[channel][3] == 1 && granule == 1 {
					// Copy scalefactors from granule 0 to granule 1
					for sfb := scfsiGroup3; sfb < consts.SfbLongBands; sfb++ {
						dst.ScalefacL[1][channel][sfb] = dst.ScalefacL[0][channel][sfb]
					}
				}
			}
			// Read Huffman coded data. Skip stuffing bits.
			if err := readHuffman(bitStream, header, sideInfo, dst, part2Start, granule, channel); err != nil {
				return err
			}
		}
	}

	return nil
}

//nolint:varnamelen // short names mirror the MPEG2 spec
func getScaleFactorsMpeg2( //nolint:gocognit // the spec's scale-factor layout; hot path
	bitStream *bits.Reader,
	header *frameheader.Header,
	sideInfo *sideinfo.SideInfo,
	dst *MainData,
) error {
	nch := header.Channels()

	for channel := range nch {
		part2Start := bitStream.BitPos()
		numbits := 0
		slen := nSlen2[sideInfo.ScalefacCompress[0][channel]]
		sideInfo.Preflag[0][channel] = (slen >> 15) & 0x1 //nolint:mnd // the packed slen's preflag bit

		blockTypeIdx := 0
		if sideInfo.BlockType[0][channel] == 2 {
			blockTypeIdx++
			if sideInfo.MixedBlockFlag[0][channel] != 0 {
				blockTypeIdx++
			}
		}

		d := (slen >> 12) & 0x7 //nolint:mnd // the packed slen's table-select field

		// Stack-local array replaces dynamic append.
		var sf [45]int

		sfIdx := 0

		for idx := range 4 {
			num := slen & 0x7 //nolint:mnd // each packed slen field is 3 bits
			slen >>= 3

			if num > 0 {
				for range scalefacSizesMpeg2[blockTypeIdx][d][idx] {
					sf[sfIdx] = bitStream.ReadBits(num)
					sfIdx++
				}

				numbits += scalefacSizesMpeg2[blockTypeIdx][d][idx] * num
			} else {
				for range scalefacSizesMpeg2[blockTypeIdx][d][idx] {
					// sf[sfIdx] is already 0
					sfIdx++
				}
			}
		}

		// Trailing zeros for block type alignment.
		trailingZeros := (blockTypeIdx << 1) + 1
		sfIdx += trailingZeros
		_ = numbits

		if sfIdx == mpeg2LongSfbCount {
			for idx := range mpeg2LongSfbCount {
				dst.ScalefacL[0][channel][idx] = sf[idx]
			}
		} else {
			for x := range mpeg2ShortSfbCount {
				for idx := range 3 {
					dst.ScalefacS[0][channel][x][idx] = sf[(x*3)+idx]
				}
			}
		}

		// Read Huffman coded data. Skip stuffing bits.
		if err := readHuffman(bitStream, header, sideInfo, dst, part2Start, 0, channel); err != nil {
			return err
		}
	}

	return nil
}
