// Package recordingcatalog 保存录播身份、路径别名及后台核验的房间统计。
package recordingcatalog

type Entry struct {
	RecordingID   string
	SourceVersion string
	Path          string
	Present       bool
}

type Room struct {
	FolderPath  string
	Platform    string
	HostName    string
	VideoCount  int
	TotalSize   int64
	DerivedSize int64
	LatestVideo string
	LatestAt    int64
	CheckedAt   int64
	Status      string
}
