package conf

var (
	LenStackBuf = 4096

	// 日志配置
	LogLevel string
	LogPath  string
	LogFlag  int

	// 控制台
	ConsolePort   int
	ConsolePrompt string = "Leaf# "
	ProfilePath   string

	// 集群
	ListenAddr      string
	ConnAddrs       []string
	PendingWriteNum int
)
