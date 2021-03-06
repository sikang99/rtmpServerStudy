package h265parser

import (
"bytes"
"fmt"
"rtmpServerStudy/utils/bits"
"rtmpServerStudy/utils/bits/pio"
"rtmpServerStudy/av"
	"encoding/hex"
)

const (
	NALU_SEI = 6
	NALU_PPS = 7
	NALU_SPS = 8
	NALU_AUD = 9
)

func IsDataNALU(b []byte) bool {
	typ := b[0] & 0x1f
	return typ >= 1 && typ <= 5
}

/*
From: http://stackoverflow.com/questions/24884827/possible-locations-for-sequence-picture-parameter-sets-for-h-264-stream

First off, it's important to understand that there is no single standard H.264 elementary bitstream format. The specification document does contain an Annex, specifically Annex B, that describes one possible format, but it is not an actual requirement. The standard specifies how video is encoded into individual packets. How these packets are stored and transmitted is left open to the integrator.

1. Annex B
Network Abstraction Layer Units
The packets are called Network Abstraction Layer Units. Often abbreviated NALU (or sometimes just NAL) each packet can be individually parsed and processed. The first byte of each NALU contains the NALU type, specifically bits 3 through 7. (bit 0 is always off, and bits 1-2 indicate whether a NALU is referenced by another NALU).

There are 19 different NALU types defined separated into two categories, VCL and non-VCL:

VCL, or Video Coding Layer packets contain the actual visual information.
Non-VCLs contain metadata that may or may not be required to decode the video.
A single NALU, or even a VCL NALU is NOT the same thing as a frame. A frame can be ‘sliced’ into several NALUs. Just like you can slice a pizza. One or more slices are then virtually grouped into a Access Units (AU) that contain one frame. Slicing does come at a slight quality cost, so it is not often used.

Below is a table of all defined NALUs.

0      Unspecified                                                    non-VCL
1      Coded slice of a non-IDR picture                               VCL
2      Coded slice data partition A                                   VCL
3      Coded slice data partition B                                   VCL
4      Coded slice data partition C                                   VCL
5      Coded slice of an IDR picture                                  VCL
6      Supplemental enhancement information (SEI)                     non-VCL
7      Sequence parameter set                                         non-VCL
8      Picture parameter set                                          non-VCL
9      Access unit delimiter                                          non-VCL
10     End of sequence                                                non-VCL
11     End of stream                                                  non-VCL
12     Filler data                                                    non-VCL
13     Sequence parameter set extension                               non-VCL
14     Prefix NAL unit                                                non-VCL
15     Subset sequence parameter set                                  non-VCL
16     Depth parameter set                                            non-VCL
17..18 Reserved                                                       non-VCL
19     Coded slice of an auxiliary coded picture without partitioning non-VCL
20     Coded slice extension                                          non-VCL
21     Coded slice extension for depth view components                non-VCL
22..23 Reserved                                                       non-VCL
24..31 Unspecified                                                    non-VCL
There are a couple of NALU types where having knowledge of may be helpful later.

Sequence Parameter Set (SPS). This non-VCL NALU contains information required to configure the decoder such as profile, level, resolution, frame rate.
Picture Parameter Set (PPS). Similar to the SPS, this non-VCL contains information on entropy coding mode, slice groups, motion prediction and deblocking filters.
Instantaneous Decoder Refresh (IDR). This VCL NALU is a self contained image slice. That is, an IDR can be decoded and displayed without referencing any other NALU save SPS and PPS.
Access Unit Delimiter (AUD). An AUD is an optional NALU that can be use to delimit frames in an elementary stream. It is not required (unless otherwise stated by the container/protocol, like TS), and is often not included in order to save space, but it can be useful to finds the start of a frame without having to fully parse each NALU.
NALU Start Codes
A NALU does not contain is its size. Therefore simply concatenating the NALUs to create a stream will not work because you will not know where one stops and the next begins.

The Annex B specification solves this by requiring ‘Start Codes’ to precede each NALU. A start code is 2 or 3 0x00 bytes followed with a 0x01 byte. e.g. 0x000001 or 0x00000001.

The 4 byte variation is useful for transmission over a serial connection as it is trivial to byte align the stream by looking for 31 zero bits followed by a one. If the next bit is 0 (because every NALU starts with a 0 bit), it is the start of a NALU. The 4 byte variation is usually only used for signaling random access points in the stream such as a SPS PPS AUD and IDR Where as the 3 byte variation is used everywhere else to save space.

Emulation Prevention Bytes
Start codes work because the four byte sequences 0x000000, 0x000001, 0x000002 and 0x000003 are illegal within a non-RBSP NALU. So when creating a NALU, care is taken to escape these values that could otherwise be confused with a start code. This is accomplished by inserting an ‘Emulation Prevention’ byte 0x03, so that 0x000001 becomes 0x00000301.

When decoding, it is important to look for and ignore emulation prevention bytes. Because emulation prevention bytes can occur almost anywhere within a NALU, it is often more convenient in documentation to assume they have already been removed. A representation without emulation prevention bytes is called Raw Byte Sequence Payload (RBSP).

Example
Let's look at a complete example.

0x0000 | 00 00 00 01 67 64 00 0A AC 72 84 44 26 84 00 00
0x0010 | 03 00 04 00 00 03 00 CA 3C 48 96 11 80 00 00 00
0x0020 | 01 68 E8 43 8F 13 21 30 00 00 01 65 88 81 00 05
0x0030 | 4E 7F 87 DF 61 A5 8B 95 EE A4 E9 38 B7 6A 30 6A
0x0040 | 71 B9 55 60 0B 76 2E B5 0E E4 80 59 27 B8 67 A9
0x0050 | 63 37 5E 82 20 55 FB E4 6A E9 37 35 72 E2 22 91
0x0060 | 9E 4D FF 60 86 CE 7E 42 B7 95 CE 2A E1 26 BE 87
0x0070 | 73 84 26 BA 16 36 F4 E6 9F 17 DA D8 64 75 54 B1
0x0080 | F3 45 0C 0B 3C 74 B3 9D BC EB 53 73 87 C3 0E 62
0x0090 | 47 48 62 CA 59 EB 86 3F 3A FA 86 B5 BF A8 6D 06
0x00A0 | 16 50 82 C4 CE 62 9E 4E E6 4C C7 30 3E DE A1 0B
0x00B0 | D8 83 0B B6 B8 28 BC A9 EB 77 43 FC 7A 17 94 85
0x00C0 | 21 CA 37 6B 30 95 B5 46 77 30 60 B7 12 D6 8C C5
0x00D0 | 54 85 29 D8 69 A9 6F 12 4E 71 DF E3 E2 B1 6B 6B
0x00E0 | BF 9F FB 2E 57 30 A9 69 76 C4 46 A2 DF FA 91 D9
0x00F0 | 50 74 55 1D 49 04 5A 1C D6 86 68 7C B6 61 48 6C
0x0100 | 96 E6 12 4C 27 AD BA C7 51 99 8E D0 F0 ED 8E F6
0x0110 | 65 79 79 A6 12 A1 95 DB C8 AE E3 B6 35 E6 8D BC
0x0120 | 48 A3 7F AF 4A 28 8A 53 E2 7E 68 08 9F 67 77 98
0x0130 | 52 DB 50 84 D6 5E 25 E1 4A 99 58 34 C7 11 D6 43
0x0140 | FF C4 FD 9A 44 16 D1 B2 FB 02 DB A1 89 69 34 C2
0x0150 | 32 55 98 F9 9B B2 31 3F 49 59 0C 06 8C DB A5 B2
0x0160 | 9D 7E 12 2F D0 87 94 44 E4 0A 76 EF 99 2D 91 18
0x0170 | 39 50 3B 29 3B F5 2C 97 73 48 91 83 B0 A6 F3 4B
0x0180 | 70 2F 1C 8F 3B 78 23 C6 AA 86 46 43 1D D7 2A 23
0x0190 | 5E 2C D9 48 0A F5 F5 2C D1 FB 3F F0 4B 78 37 E9
0x01A0 | 45 DD 72 CF 80 35 C3 95 07 F3 D9 06 E5 4A 58 76
0x01B0 | 03 6C 81 20 62 45 65 44 73 BC FE C1 9F 31 E5 DB
0x01C0 | 89 5C 6B 79 D8 68 90 D7 26 A8 A1 88 86 81 DC 9A
0x01D0 | 4F 40 A5 23 C7 DE BE 6F 76 AB 79 16 51 21 67 83
0x01E0 | 2E F3 D6 27 1A 42 C2 94 D1 5D 6C DB 4A 7A E2 CB
0x01F0 | 0B B0 68 0B BE 19 59 00 50 FC C0 BD 9D F5 F5 F8
0x0200 | A8 17 19 D6 B3 E9 74 BA 50 E5 2C 45 7B F9 93 EA
0x0210 | 5A F9 A9 30 B1 6F 5B 36 24 1E 8D 55 57 F4 CC 67
0x0220 | B2 65 6A A9 36 26 D0 06 B8 E2 E3 73 8B D1 C0 1C
0x0230 | 52 15 CA B5 AC 60 3E 36 42 F1 2C BD 99 77 AB A8
0x0240 | A9 A4 8E 9C 8B 84 DE 73 F0 91 29 97 AE DB AF D6
0x0250 | F8 5E 9B 86 B3 B3 03 B3 AC 75 6F A6 11 69 2F 3D
0x0260 | 3A CE FA 53 86 60 95 6C BB C5 4E F3

This is a complete AU containing 3 NALUs. As you can see, we begin with a Start code followed by an SPS (SPS starts with 67). Within the SPS, you will see two Emulation Prevention bytes. Without these bytes the illegal sequence 0x000000 would occur at these positions. Next you will see a start code followed by a PPS (PPS starts with 68) and one final start code followed by an IDR slice. This is a complete H.264 stream. If you type these values into a hex editor and save the file with a .264 extension, you will be able to convert it to this image:

Lena

Annex B is commonly used in live and streaming formats such as transport streams, over the air broadcasts, and DVDs. In these formats it is common to repeat the SPS and PPS periodically, usually preceding every IDR thus creating a random access point for the decoder. This enables the ability to join a stream already in progress.

2. AVCC
The other common method of storing an H.264 stream is the AVCC format. In this format, each NALU is preceded with its length (in big endian format). This method is easier to parse, but you lose the byte alignment features of Annex B. Just to complicate things, the length may be encoded using 1, 2 or 4 bytes. This value is stored in a header object. This header is often called ‘extradata’ or ‘sequence header’. Its basic format is as follows:

bits
8   version ( always 0x01 )
8   avc profile ( sps[0][1] )
8   avc compatibility ( sps[0][2] )
8   avc level ( sps[0][3] )
6   reserved ( all bits on )
2   NALULengthSizeMinusOne
3   reserved ( all bits on )
5   number of SPS NALUs (usually 1)
repeated once per SPS:
  16         SPS size
	variable   SPS NALU data
8   number of PPS NALUs (usually 1)
repeated once per PPS
  16         PPS size
  variable   PPS NALU data

Using the same example above, the AVCC extradata will look like this:

0x0000 | 01 64 00 0A FF E1 00 19 67 64 00 0A AC 72 84 44
0x0010 | 26 84 00 00 03 00 04 00 00 03 00 CA 3C 48 96 11
0x0020 | 80 01 00 07 68 E8 43 8F 13 21 30

You will notice SPS and PPS is now stored out of band. That is, separate from the elementary stream data. Storage and transmission of this data is the job of the file container, and beyond the scope of this document. Notice that even though we are not using start codes, emulation prevention bytes are still inserted.

Additionally, there is a new variable called NALULengthSizeMinusOne. This confusingly named variable tells us how many bytes to use to store the length of each NALU. So, if NALULengthSizeMinusOne is set to 0, then each NALU is preceded with a single byte indicating its length. Using a single byte to store the size, the max size of a NALU is 255 bytes. That is obviously pretty small. Way too small for an entire key frame. Using 2 bytes gives us 64k per NALU. It would work in our example, but is still a pretty low limit. 3 bytes would be perfect, but for some reason is not universally supported. Therefore, 4 bytes is by far the most common, and it is what we used here:

0x0000 | 00 00 02 41 65 88 81 00 05 4E 7F 87 DF 61 A5 8B
0x0010 | 95 EE A4 E9 38 B7 6A 30 6A 71 B9 55 60 0B 76 2E
0x0020 | B5 0E E4 80 59 27 B8 67 A9 63 37 5E 82 20 55 FB
0x0030 | E4 6A E9 37 35 72 E2 22 91 9E 4D FF 60 86 CE 7E
0x0040 | 42 B7 95 CE 2A E1 26 BE 87 73 84 26 BA 16 36 F4
0x0050 | E6 9F 17 DA D8 64 75 54 B1 F3 45 0C 0B 3C 74 B3
0x0060 | 9D BC EB 53 73 87 C3 0E 62 47 48 62 CA 59 EB 86
0x0070 | 3F 3A FA 86 B5 BF A8 6D 06 16 50 82 C4 CE 62 9E
0x0080 | 4E E6 4C C7 30 3E DE A1 0B D8 83 0B B6 B8 28 BC
0x0090 | A9 EB 77 43 FC 7A 17 94 85 21 CA 37 6B 30 95 B5
0x00A0 | 46 77 30 60 B7 12 D6 8C C5 54 85 29 D8 69 A9 6F
0x00B0 | 12 4E 71 DF E3 E2 B1 6B 6B BF 9F FB 2E 57 30 A9
0x00C0 | 69 76 C4 46 A2 DF FA 91 D9 50 74 55 1D 49 04 5A
0x00D0 | 1C D6 86 68 7C B6 61 48 6C 96 E6 12 4C 27 AD BA
0x00E0 | C7 51 99 8E D0 F0 ED 8E F6 65 79 79 A6 12 A1 95
0x00F0 | DB C8 AE E3 B6 35 E6 8D BC 48 A3 7F AF 4A 28 8A
0x0100 | 53 E2 7E 68 08 9F 67 77 98 52 DB 50 84 D6 5E 25
0x0110 | E1 4A 99 58 34 C7 11 D6 43 FF C4 FD 9A 44 16 D1
0x0120 | B2 FB 02 DB A1 89 69 34 C2 32 55 98 F9 9B B2 31
0x0130 | 3F 49 59 0C 06 8C DB A5 B2 9D 7E 12 2F D0 87 94
0x0140 | 44 E4 0A 76 EF 99 2D 91 18 39 50 3B 29 3B F5 2C
0x0150 | 97 73 48 91 83 B0 A6 F3 4B 70 2F 1C 8F 3B 78 23
0x0160 | C6 AA 86 46 43 1D D7 2A 23 5E 2C D9 48 0A F5 F5
0x0170 | 2C D1 FB 3F F0 4B 78 37 E9 45 DD 72 CF 80 35 C3
0x0180 | 95 07 F3 D9 06 E5 4A 58 76 03 6C 81 20 62 45 65
0x0190 | 44 73 BC FE C1 9F 31 E5 DB 89 5C 6B 79 D8 68 90
0x01A0 | D7 26 A8 A1 88 86 81 DC 9A 4F 40 A5 23 C7 DE BE
0x01B0 | 6F 76 AB 79 16 51 21 67 83 2E F3 D6 27 1A 42 C2
0x01C0 | 94 D1 5D 6C DB 4A 7A E2 CB 0B B0 68 0B BE 19 59
0x01D0 | 00 50 FC C0 BD 9D F5 F5 F8 A8 17 19 D6 B3 E9 74
0x01E0 | BA 50 E5 2C 45 7B F9 93 EA 5A F9 A9 30 B1 6F 5B
0x01F0 | 36 24 1E 8D 55 57 F4 CC 67 B2 65 6A A9 36 26 D0
0x0200 | 06 B8 E2 E3 73 8B D1 C0 1C 52 15 CA B5 AC 60 3E
0x0210 | 36 42 F1 2C BD 99 77 AB A8 A9 A4 8E 9C 8B 84 DE
0x0220 | 73 F0 91 29 97 AE DB AF D6 F8 5E 9B 86 B3 B3 03
0x0230 | B3 AC 75 6F A6 11 69 2F 3D 3A CE FA 53 86 60 95
0x0240 | 6C BB C5 4E F3

An advantage to this format is the ability to configure the decoder at the start and jump into the middle of a stream. This is a common use case where the media is available on a random access medium such as a hard drive, and is therefore used in common container formats such as MP4 and MKV.
*/

const (
	HEVC_NAL_TRAIL_N    = 0
	HEVC_NAL_TRAIL_R    = 1
	HEVC_NAL_TSA_N      = 2
	HEVC_NAL_TSA_R      = 3
	HEVC_NAL_STSA_N     = 4
	HEVC_NAL_STSA_R     = 5
	HEVC_NAL_RADL_N     = 6
	HEVC_NAL_RADL_R     = 7
	HEVC_NAL_RASL_N     = 8
	HEVC_NAL_RASL_R     = 9
	HEVC_NAL_BLA_W_LP   = 16
	HEVC_NAL_BLA_W_RADL = 17
	HEVC_NAL_BLA_N_LP   = 18
	HEVC_NAL_IDR_W_RADL = 19
	HEVC_NAL_IDR_N_LP   = 20
	HEVC_NAL_CRA_NUT    = 21
	HEVC_NAL_VPS        = 32
	HEVC_NAL_SPS        = 33
	HEVC_NAL_PPS        = 34
	HEVC_NAL_AUD        = 35
	HEVC_NAL_EOS_NUT    = 36
	HEVC_NAL_EOB_NUT    = 37
	HEVC_NAL_FD_NUT     = 38
	HEVC_NAL_SEI_PREFIX = 39
	HEVC_NAL_SEI_SUFFIX = 40
)

var StartCodeBytes = []byte{0, 0, 1}
var AUDBytes = []byte{0, 0, 0, 1, 0x46, 0x01,0x10, 0, 0, 0, 1} // AUD

func CheckNALUsType(b []byte) (typ int) {
	_, typ = SplitNALUs(b)
	return
}

const (
	NALU_RAW = iota
	NALU_AVCC
	NALU_ANNEXB
)

func SplitNALUs(b []byte) (nalus [][]byte, typ int) {
	if len(b) < 4 {
		return [][]byte{b}, NALU_RAW
	}

	val3 := pio.U24BE(b)
	val4 := pio.U32BE(b)

	// maybe AVCC
	if val4 <= uint32(len(b)) {
		_val4 := val4
		_b := b[4:]
		nalus := [][]byte{}
		for {
			nalus = append(nalus, _b[:_val4])
			_b = _b[_val4:]
			if len(_b) < 4 {
				break
			}
			_val4 = pio.U32BE(_b)
			_b = _b[4:]
			if _val4 > uint32(len(_b)) {
				break
			}
		}
		if len(_b) == 0 {
			return nalus, NALU_AVCC
		}
	}

	// is Annex B
	if val3 == 1 || val4 == 1 {
		_val3 := val3
		_val4 := val4
		start := 0
		pos := 0
		for {
			if start != pos {
				nalus = append(nalus, b[start:pos])
			}
			if _val3 == 1 {
				pos += 3
			} else if _val4 == 1 {
				pos += 4
			}
			start = pos
			if start == len(b) {
				break
			}
			_val3 = 0
			_val4 = 0
			for pos < len(b) {
				if pos+2 < len(b) && b[pos] == 0 {
					_val3 = pio.U24BE(b[pos:])
					if _val3 == 0 {
						if pos+3 < len(b) {
							_val4 = uint32(b[pos+3])
							if _val4 == 1 {
								break
							}
						}
					} else if _val3 == 1 {
						break
					}
					pos++
				} else {
					pos++
				}
			}
		}
		typ = NALU_ANNEXB
		return
	}

	return [][]byte{b}, NALU_RAW
}

type SPSInfo struct {
	ProfileIdc uint
	LevelIdc   uint
	VpsId      uint
	MaxSubLayers uint
	SpsId      uint
	CfIdc	   uint
	MbWidth  uint
	MbHeight uint

	CropLeft   uint
	CropRight  uint
	CropTop    uint
	CropBottom uint

	Width  uint
	Height uint
}

const(
	MAX_VPS_COUNT  = 16
	MAX_SUB_LAYERS = 7
	MAX_SPS_COUNT  = 32
)

func decodeProfileTierLevel(r *bits.GolombBitReader)(err error){

	/*
		profile_space
	*/
	if _,err = r.ReadBits(2);err != nil{
		return 
	}
	/*
		tier_flag
	*/
	if _,err = r.ReadBits(1);err != nil{
		return 
	}
	/*profile_idc*/
	if _,err = r.ReadBits(5);err != nil{
		return 
	}


	/*profile_compatibility_flag*/

	if _,err = r.ReadBits(32);err != nil{
		return
	}

	/*reserved 44bit*/
	if _,err = r.ReadBits(48);err != nil{
		return 
	}

	return 
}

func CodecParsePtl(r *bits.GolombBitReader,MaxSubLayers uint)(err error){
	var n uint;
	profilePresentFlag := make([]uint,MAX_SUB_LAYERS)
	levelPresentFlag := make([]uint,MAX_SUB_LAYERS)
	
	if err = decodeProfileTierLevel(r);err != nil{
		return
	}
		
	if n,err = r.ReadBits(8);err != nil{
		return 
	}

	for n=0;n < MaxSubLayers; n++{

		if profilePresentFlag[n],err = r.ReadBit();err != nil{
			return 
		}
		if levelPresentFlag[n],err = r.ReadBit();err != nil{
			return 
		}
	}
	
	if MaxSubLayers >0{
		for n = MaxSubLayers;n < 8; n++{
 	   		if _,err = r.ReadBits(2);err != nil{
        			return
    			}
		}
	}
	
	for n=0;n < MaxSubLayers; n++{

		if (profilePresentFlag[n] >0){
			if err = decodeProfileTierLevel(r);err != nil{
				return
			}
		}

		if levelPresentFlag[n] >0 {
			if _,err = r.ReadBits(8);err != nil{
				return
            		}
		}
	}

	return	
}

func ParseSPS(data []byte) (self SPSInfo, err error) {
	r := &bits.GolombBitReader{R: bytes.NewReader(data)}
	var nalType uint
	var tmp  uint

	if _,err=r.ReadBit();err!=nil{
		return 
	}

	if nalType,err=r.ReadBits(6);err!= nil{
		return 
	}

	if nalType != HEVC_NAL_SPS{
		err =  ErrDecconfInvalid
		return
	}

	if _,err = r.ReadBits(9);err != nil{
		return 
	}

	if self.VpsId,err = r.ReadBits(4);err != nil{
		return
	}
	
	if self.MaxSubLayers,err = r.ReadBits(3);err != nil{
       	 	return
    	}

	if _,err = r.ReadBit();err != nil{
        	return
    	}

	if err = CodecParsePtl(r,self.MaxSubLayers);err != nil{
		return 
	}

	if _,err = r.ReadBits(4);err != nil{
		return
	}

	if self.SpsId, err = r.ReadExponentialGolombCode(); err != nil {
		return
	}

	if self.SpsId >= MAX_SPS_COUNT{
		err =  ErrDecconfInvalid 
		return
	}
	
	if self.CfIdc, err = r.ReadExponentialGolombCode(); err != nil {
		return
	}

	if self.CfIdc == 3{
		if _,err = r.ReadBit();err != nil{
        		return
    		}
	}

	if self.Width ,err =  r.ReadExponentialGolombCode();err != nil {
		return
	}

	if self.Height ,err =  r.ReadExponentialGolombCode();err != nil {
		return
	}

	if tmp,err = r.ReadBit();err != nil{
            return
    }

	if (tmp >0 ){
				
		if self.CropLeft ,err =  r.ReadExponentialGolombCode();err != nil {
			return
		}

		if self.CropRight ,err =  r.ReadExponentialGolombCode();err != nil {
			return
		}

		if self.CropTop ,err =  r.ReadExponentialGolombCode();err != nil {
			return
		}
		if self.CropBottom ,err =  r.ReadExponentialGolombCode();err != nil {
			return
		}
	}else{
		self.CropLeft = 0
		self.CropRight = 0
		self.CropTop = 0
		self.CropBottom = 0
	}

	self.Width = self.Width - (self.CropLeft + self.CropRight)
	self.Height = self.Height - (self.CropRight + self.CropBottom)

	return
}

type CodecData struct {
	Record     []byte
	RecordInfo AVCDecoderConfRecord
	SPSInfo    SPSInfo
}

func (self CodecData) Type() av.CodecType {
	return av.H265
}

func (self CodecData) AVCDecoderConfRecordBytes() []byte {
	return self.Record
}

func (self CodecData) SPS() []byte {
	return self.RecordInfo.SPS[0]
}

func (self CodecData) PPS() []byte {
	return self.RecordInfo.PPS[0]
}

func (self CodecData) Width() int {
	return int(self.SPSInfo.Width)
}

func (self CodecData) Height() int {
	return int(self.SPSInfo.Height)
}

func NewCodecDataFromAVCDecoderConfRecord(record []byte) (self CodecData, err error) {
	self.Record = record
	fmt.Print(hex.Dump(record))
	if _, err = (&self.RecordInfo).Unmarshal(record); err != nil {
		fmt.Println(err)
		return
	}
	if len(self.RecordInfo.SPS) == 0 {
		err = fmt.Errorf("%s","H265Parser.No.SPS.Found.AVCDecoderConfRecord")
		fmt.Println(err)
		return
	}
	if len(self.RecordInfo.PPS) == 0 {
		err = fmt.Errorf("%s","H265Parser.No.PPS.Found.AVCDecoderConfRecord")
		fmt.Println(err)
		return
	}
	fmt.Print(hex.Dump(self.RecordInfo.SPS[0]))
	if self.SPSInfo, err = ParseSPS(self.RecordInfo.SPS[0]); err != nil {
		err = fmt.Errorf("H265Parser.Parse.SPS.Failed(%s)", err)
		fmt.Println(err)
		return
	}
	return
}

func NewCodecDataFromSPSAndPPS(sps, pps [][]byte) (self CodecData, err error) {
	recordinfo := AVCDecoderConfRecord{}
	recordinfo.AVCProfileIndication = sps[0][1]
	recordinfo.ProfileCompatibility = uint32(pio.U32BE(sps[0][2:]))
	recordinfo.AVCLevelIndication = sps[0][3]
	recordinfo.SPS = sps
	recordinfo.PPS = pps
	recordinfo.LengthSizeMinusOne = 3

	buf := make([]byte, recordinfo.Len())
	recordinfo.Marshal(buf)

	self.RecordInfo = recordinfo
	self.Record = buf

	if self.SPSInfo, err = ParseSPS(sps[0]); err != nil {
		return
	}
	return
}

type AVCDecoderConfRecord struct {
	AVCProfileIndication uint8
	ProfileCompatibility uint32
	AVCLevelIndication   uint8
	LengthSizeMinusOne   uint8
	VPS                  [][]byte
	SPS                  [][]byte
	PPS                  [][]byte
}

var ErrDecconfInvalid = fmt.Errorf("%s","H264Parser.AVCDecoderConfRecord.invalid")

func (self *AVCDecoderConfRecord) Unmarshal(b []byte) (n int, err error) {
	b_len:=len(b)	
	if len(b) < 7 {
		err = ErrDecconfInvalid
		return
	}
	
	n++
	self.AVCProfileIndication = b[1] & 0x1f
	n++
	self.ProfileCompatibility =  uint32(pio.U32BE(b[n:]))
	n+=4
	self.AVCLevelIndication = b[n]
	n++
	fmt.Println(self.AVCProfileIndication, self.ProfileCompatibility,self.AVCLevelIndication)
	if n+15 >= b_len{
		err = ErrDecconfInvalid
		return
	}
	n += 15
	spscount :=int(b[n])
	n++;
	fmt.Println("spscount:%d,,%d",spscount,len(b))	
	nal_type := 0
	
	for i := 0; i < spscount; i++ {
		nal_type = int(b[n] & 0x3f)
		n++

		if b_len < n+2 {
			err = ErrDecconfInvalid
			return
		}

		nal_num:= int(pio.U16BE(b[n:]))
		n += 2
		for m:=0;m < nal_num;m++{
			nal_len:= int(pio.U16BE(b[n:]))
			n += 2
			if b_len < (n + nal_len){
				err = ErrDecconfInvalid
				return
			}
			switch nal_type {
				case     HEVC_NAL_VPS:
					self.VPS = append(self.VPS, b[n:n+nal_len])
    			case 	 HEVC_NAL_SPS:
					self.SPS = append(self.SPS, b[n:n+nal_len])
    			case     HEVC_NAL_PPS:
					self.PPS = append(self.PPS, b[n:n+nal_len])
			}
			n += nal_len
		}
	}
	fmt.Print("VPS:",hex.Dump(self.VPS[0]))
	fmt.Print("SPS:",hex.Dump(self.SPS[0]))
	fmt.Print("PPS:",hex.Dump(self.PPS[0]))
	return
}

func (self AVCDecoderConfRecord) Len() (n int) {
	n = 7
	for _, sps := range self.SPS {
		n += 2 + len(sps)
	}
	for _, pps := range self.PPS {
		n += 2 + len(pps)
	}
	return
}

func (self AVCDecoderConfRecord) Marshal(b []byte) (n int) {
	
	b[n] = 1
	n++
	b[n] = self.AVCProfileIndication
	n++
	pio.PutU32BE(b[n:],self.ProfileCompatibility)
	n+=4
	b[n] = self.AVCLevelIndication
	//n += 6

	for _, sps := range self.SPS {
		pio.PutU16BE(b[n:], uint16(len(sps)))
		n += 2
		copy(b[n:], sps)
		n += len(sps)
	}

	b[n] = uint8(len(self.PPS))
	n++

	for _, pps := range self.PPS {
		pio.PutU16BE(b[n:], uint16(len(pps)))
		n += 2
		copy(b[n:], pps)
		n += len(pps)
	}

	return
}

type SliceType uint

func (self SliceType) String() string {
	switch self {
	case SLICE_P:
		return "P"
	case SLICE_B:
		return "B"
	case SLICE_I:
		return "I"
	}
	return ""
}

const (
	SLICE_P = iota + 1
	SLICE_B
	SLICE_I
)

func ParseSliceHeaderFromNALU(packet []byte) (sliceType SliceType, err error) {

	if len(packet) <= 1 {
		err = fmt.Errorf("%s","H264Parser.Packet.Too.Short.To.Parse.Slice.Header")
		return
	}

	nal_unit_type := packet[0] & 0x1f
	switch nal_unit_type {
	case 1, 2, 5, 19:
	// slice_layer_without_partitioning_rbsp
	// slice_data_partition_a_layer_rbsp

	default:
		err = fmt.Errorf("h264parser.nal_unit_type=%d Has.No.Slice.Header", nal_unit_type)
		return
	}

	r := &bits.GolombBitReader{R: bytes.NewReader(packet[1:])}

	// first_mb_in_slice
	if _, err = r.ReadExponentialGolombCode(); err != nil {
		return
	}

	// slice_type
	var u uint
	if u, err = r.ReadExponentialGolombCode(); err != nil {
		return
	}

	switch u {
	case 0, 3, 5, 8:
		sliceType = SLICE_P
	case 1, 6:
		sliceType = SLICE_B
	case 2, 4, 7, 9:
		sliceType = SLICE_I
	default:
		err = fmt.Errorf("H264Parser.Slice_type=%d.Invalid", u)
		return
	}

	return
}
