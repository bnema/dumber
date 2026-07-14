package libwebp

type VP8StatusCode int32

const (
	VP8StatusOK              VP8StatusCode = 0
	VP8StatusOutOfMemory     VP8StatusCode = 1
	VP8StatusInvalidParam    VP8StatusCode = 2
	VP8StatusBitstreamError  VP8StatusCode = 3
	VP8StatusUnsupportedFeat VP8StatusCode = 4
	VP8StatusSuspended       VP8StatusCode = 5
	VP8StatusUserAbort       VP8StatusCode = 6
	VP8StatusNotEnoughData   VP8StatusCode = 7
	WebPDecoderABIVersion    int32         = 0x0210
	WebPEncoderABIVersion    int32         = 0x0210
)

type WebPBitstreamFeatures struct {
	Width        int32
	Height       int32
	HasAlpha     int32
	HasAnimation int32
	Format       int32
	Pad          [5]uint32
}

type WebPRGBABuffer struct {
	RGBA   uintptr
	Stride int32
	_      int32
	Size   uintptr
}

type WebPYUVABuffer struct {
	Y       uintptr
	U       uintptr
	V       uintptr
	A       uintptr
	YStride int32
	UStride int32
	VStride int32
	AStride int32
	YSize   uintptr
	USize   uintptr
	VSize   uintptr
	ASize   uintptr
}

// WebPDecBuffer matches the C layout used by decode.h.
// The union field is represented as raw bytes and should be manipulated only
// through dedicated helper functions/wrappers.
type WebPDecBuffer struct {
	Colorspace       int32
	Width            int32
	Height           int32
	IsExternalMemory int32
	BufferUnion      [80]byte
	Pad              [4]uint32
	PrivateMemory    uintptr
}

type WebPDecoderOptions struct {
	BypassFiltering        int32
	NoFancyUpsampling      int32
	UseCropping            int32
	CropLeft               int32
	CropTop                int32
	CropWidth              int32
	CropHeight             int32
	UseScaling             int32
	ScaledWidth            int32
	ScaledHeight           int32
	UseThreads             int32
	DitheringStrength      int32
	Flip                   int32
	AlphaDitheringStrength int32
	Pad                    [5]uint32
}

type WebPDecoderConfig struct {
	Input   WebPBitstreamFeatures
	Output  WebPDecBuffer
	Options WebPDecoderOptions
}

type WebPConfig struct {
	Lossless         int32
	Quality          float32
	Method           int32
	ImageHint        int32
	TargetSize       int32
	TargetPSNR       float32
	Segments         int32
	SnsStrength      int32
	FilterStrength   int32
	FilterSharpness  int32
	FilterType       int32
	Autofilter       int32
	AlphaCompression int32
	AlphaFiltering   int32
	AlphaQuality     int32
	Pass             int32
	ShowCompressed   int32
	Preprocessing    int32
	Partitions       int32
	PartitionLimit   int32
	EmulateJpegSize  int32
	ThreadLevel      int32
	LowMemory        int32
	NearLossless     int32
	Exact            int32
	UseDeltaPalette  int32
	UseSharpYuv      int32
	QMin             int32
	QMax             int32
}

type WebPMemoryWriter struct {
	Mem     uintptr
	Size    uintptr
	MaxSize uintptr
	Pad     [1]uint32
}

type WebPPicture struct {
	UseArgb    int32
	Colorspace int32
	Width      int32
	Height     int32
	Y          uintptr
	U          uintptr
	V          uintptr
	YStride    int32
	UvStride   int32
	A          uintptr
	AStride    int32
	Pad1       [2]uint32

	Argb       uintptr
	ArgbStride int32
	Pad2       [3]uint32

	Writer    uintptr
	CustomPtr uintptr

	ExtraInfoType int32
	ExtraInfo     uintptr

	Stats uintptr

	ErrorCode    int32
	ProgressHook uintptr
	UserData     uintptr

	Pad3 [3]uint32

	Pad4 uintptr
	Pad5 uintptr
	Pad6 [8]uint32

	Memory     uintptr
	MemoryArgb uintptr
	Pad7       [2]uintptr
}
