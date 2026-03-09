package main

import "html/template"

// --- Index page ---

type indexData struct {
	SingleZIM bool
	NoInfo    bool
	Entries   []indexEntry
}

type indexEntry struct {
	Slug        string
	Title       string
	Description string
	Filename    string
	Language    string
	Creator     string
	Flavour     string
	Date        string
	EntryCount  int
}

// --- Search page ---

type searchData struct {
	Slug     string
	Title    string
	Query    string
	HasQuery bool
	Results  []searchResult
}

// --- Browse page ---

type browseData struct {
	Slug        string
	Title       string
	TotalC      int
	Letters     []browseLetterInfo
	HashActive  bool
	HasLetter   bool
	Letter      string
	LetterCount int
	Entries     []searchResult
	NoEntries   bool
	HasPrev     bool
	HasNext     bool
	PrevOffset  int
	NextOffset  int
	Limit       int
	PageStart   int
	PageEnd     int
}

type browseLetterInfo struct {
	L      string
	Empty  bool
	Active bool
}

// --- Docs page ---

type docsData struct {
	Content template.HTML
}

// --- Info dashboard ---

type infoData struct {
	Slug          string
	Title         string
	Filename      string
	UUID          string
	MajorVersion  uint16
	MinorVersion  uint16
	EntryCount    uint32
	ClusterCount  uint32
	IsSplit       bool
	SplitParts    []infoSplitPart
	SplitTotal    string
	Cache         infoCacheData
	HasMainEntry  bool
	MainPath      string
	MainTitle     string
	FulltextIndex infoIndexData
	TitleIndex    infoIndexData
	Metadata      []infoMetaRow
	Namespaces    []infoNSRow
	MIMECounts    []infoMIMECountRow
	RedirectCount int
	MIMETypes     []infoMIMETypeRow
}

type infoSplitPart struct {
	Filename string
	Size     string
}

type infoCacheData struct {
	Size     int
	Capacity int
	HitRate  string
	Hits     int64
	Misses   int64
	Bytes    string
}

type infoIndexData struct {
	Has    bool
	Format string
}

type infoMetaRow struct {
	Key   string
	Value string
	Long  bool
}

type infoNSRow struct {
	NS    string
	Desc  string
	Count int
}

type infoMIMECountRow struct {
	MIME  string
	Count int
}

type infoMIMETypeRow struct {
	Index int
	MIME  string
}

// --- Namespace browser ---

type infoNSData struct {
	Slug       string
	Title      string
	NS         string
	TypeFilter string
	MIMETypes  []string
	Total      int
	Rows       []infoNSRowItem
	HasPrev    bool
	HasNext    bool
	PrevOffset int
	NextOffset int
	Limit      int
}

type infoNSRowItem struct {
	Index      uint32
	Path       string
	FullPath   string
	Title      string
	IsRedirect bool
	IsContent  bool
	MIME       string
}

// --- MIME browser ---

type infoMIMEData struct {
	Slug       string
	Title      string
	Heading    string
	MIMEFilter string
	Total      int
	Matches    []infoMIMEMatch
	HasPrev    bool
	HasNext    bool
	PrevOffset int
	NextOffset int
	Limit      int
}

type infoMIMEMatch struct {
	Index       uint32
	Path        string
	FullPath    string
	Title       string
	ContentLink string
}

// --- Entry detail ---

type infoEntryData struct {
	Slug           string
	ArchiveTitle   string
	EntryIdx       uint64
	FullPath       string
	NS             string
	Path           string
	EntryTitle     string
	IsRedirect     bool
	RedirectTarget *entryRef
	ResolvesTo     *entryRef
	MIME           string
	HasSize        bool
	ContentSize    string
	ViewLink       string
	HasPrev        bool
	HasNext        bool
	PrevIdx        uint64
	NextIdx        uint64
}

type entryRef struct {
	Index    uint32
	FullPath string
}

// --- Cluster list ---

type infoClusterListData struct {
	Slug       string
	Title      string
	Total      int
	Rows       []infoClusterRow
	HasPrev    bool
	HasNext    bool
	PrevOffset int
	NextOffset int
	Limit      int
}

type infoClusterRow struct {
	Num      uint32
	Offset   uint64
	Size     string
	Comp     string
	Extended bool
	HasError bool
	ErrMsg   string
}

// --- Cluster detail ---

type infoClusterDetailData struct {
	Slug           string
	ArchiveTitle   string
	N              uint32
	ClusterCount   uint32
	Offset         uint64
	CompressedSize string
	Compression    string
	Extended       bool
	BlobError      string
	BlobCount      int
	TotalDecomp    string
	Blobs          []infoBlob
	EntryError     string
	Entries        []infoClusterEntry
	HasPrev        bool
	HasNext        bool
	PrevN          uint32
	NextN          uint32
}

type infoBlob struct {
	Index int
	Size  string
}

type infoClusterEntry struct {
	Index     uint32
	FullPath  string
	Path      string
	Title     string
	IsContent bool
}

// --- Bar (injected into ZIM pages) ---

type barLetterInfo struct {
	L      string
	Active bool
}

// --- Error page ---

type errorPageData struct {
	Status     int
	StatusText string
	Icon       template.HTML
	Heading    string
	Detail     string
}

// --- Server info page (/_info) ---

type serverInfoData struct {
	Uptime        string
	Started       string
	GoVersion     string
	GOOS          string
	GOARCH        string
	Goroutines    int
	HeapInuse     string
	HeapIdle      string
	Sys           string
	NumGC         uint32
	ZIMs          []serverInfoZIMRow
	ZIMCount      int
	TotEntries    int
	TotCacheNow   int
	TotCacheCap   int
	TotCacheBytes string
	TotHitRate    string
	NoInfo        bool
	BuildInfo     bool
	BuildRows     []serverInfoBuildRow
	Deps          []serverInfoDep
}

type serverInfoZIMRow struct {
	Slug          string
	Title         string
	EntryCount    int
	CacheNow      int
	CacheCap      int
	CacheVal      float64 // hit rate % for sort
	CacheStr      string
	CacheHitRate  string
	CacheBytes    string
	CacheBytesRaw int64 // raw bytes for sort
}

type serverInfoBuildRow struct {
	Label string
	Value string
}

type serverInfoDep struct {
	Path    string
	Version string
	Replace string
}
