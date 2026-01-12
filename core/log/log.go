package log

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// levels
const (
	debugLevel   = 0
	releaseLevel = 1
	errorLevel   = 2
	fatalLevel   = 3
)

const (
	printDebugLevel   = "[debug  ] "
	printReleaseLevel = "[release] "
	printErrorLevel   = "[error  ] "
	printFatalLevel   = "[fatal  ] "
)

type Logger struct {
	level      int
	baseLogger *log.Logger
	baseFile   *os.File
	logPath    string // 记录日志路径用于清理过期文件
	currentDay string // 记录当前日期，格式为 YYYYMMDD
}

func New(strLevel string, pathname string, flag int) (*Logger, error) {
	// level
	var level int
	switch strings.ToLower(strLevel) {
	case "debug":
		level = debugLevel
	case "release":
		level = releaseLevel
	case "error":
		level = errorLevel
	case "fatal":
		level = fatalLevel
	default:
		return nil, errors.New("unknown level: " + strLevel)
	}

	// logger
	var baseLogger *log.Logger
	var baseFile *os.File
	var logPath string
	if pathname != "" {
		// 创建路径（如果不存在）
		if err := os.MkdirAll(pathname, 0755); err != nil {
			return nil, err
		}

		logPath = pathname // 记录日志路径
		// 注意：不再立即创建日志文件，而是在第一次写日志时创建
		baseLogger = log.New(os.Stdout, "", flag) // 临时使用标准输出，直到第一次写日志时再切换到文件
	} else {
		baseLogger = log.New(os.Stdout, "", flag)
	}

	// new
	logger := new(Logger)
	logger.level = level
	logger.baseLogger = baseLogger
	logger.baseFile = baseFile
	logger.logPath = logPath
	logger.currentDay = "" // 初始时没有设置日期

	// 清理超过30天的日志文件
	if logPath != "" {
		go logger.cleanupOldLogs()
	}

	return logger, nil
}

// checkAndSwitchLogFile 检查是否是新的一天，如果是则切换到新的日志文件
func (logger *Logger) checkAndSwitchLogFile() {
	if logger.logPath == "" {
		return // 如果没有设置日志路径，直接返回
	}

	now := time.Now()
	currentDay := fmt.Sprintf("%d%02d%02d", now.Year(), now.Month(), now.Day())

	// 如果是新的一天或还没有设置当前日期，则切换到新的日志文件
	if logger.currentDay == "" || logger.currentDay != currentDay {
		// 在创建新日志文件之前清理过期文件
		logger.cleanupOldLogs()

		// 关闭当前文件
		if logger.baseFile != nil {
			logger.baseFile.Close()
		}

		// 设置新的日期
		logger.currentDay = currentDay

		// 创建新的日志文件
		filename := fmt.Sprintf("%s.log", currentDay)
		filepath := path.Join(logger.logPath, filename)

		// 打开新文件
		file, err := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			// 如果无法创建新文件，输出到标准错误
			fmt.Fprintf(os.Stderr, "Failed to create log file: %v\n", err)
			return
		}

		// 更新logger的文件句柄和日志器
		logger.baseFile = file
		logger.baseLogger = log.New(file, "", log.LstdFlags)
	}
}

// cleanupOldLogs 清理超过30天的日志文件
func (logger *Logger) cleanupOldLogs() {
	if logger.logPath == "" {
		return
	}

	// 获取目录中的所有文件
	files, err := ioutil.ReadDir(logger.logPath)
	if err != nil {
		return
	}

	// 编译日期格式的正则表达式 (YYYYMMDD.log)
	datePattern := regexp.MustCompile(`^(\d{4})(\d{2})(\d{2})\.log$`)

	cutoffDate := time.Now().AddDate(0, 0, -30) // 30天前

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		matches := datePattern.FindStringSubmatch(file.Name())
		if len(matches) == 4 {
			year := strings.TrimPrefix(matches[1], "0")
			month := strings.TrimPrefix(matches[2], "0")
			day := strings.TrimPrefix(matches[3], "0")

			parsedTime, err := time.Parse("2006 1 2", year+" "+month+" "+day)
			if err != nil {
				continue
			}

			// 如果文件的日期早于30天前，则删除它
			if parsedTime.Before(cutoffDate) {
				filePath := filepath.Join(logger.logPath, file.Name())
				os.Remove(filePath)
			}
		}
	}
}

func (logger *Logger) Close() {
	if logger.baseFile != nil {
		logger.baseFile.Close()
	}

	logger.baseLogger = nil
	logger.baseFile = nil
}

func (logger *Logger) doPrintf(level int, printLevel string, format string, a ...interface{}) {
	if level < logger.level {
		return
	}

	// 检查是否需要切换到新的日志文件（按天）
	logger.checkAndSwitchLogFile()

	if logger.baseLogger == nil {
		panic("logger closed")
	}

	format = printLevel + format
	logger.baseLogger.Output(3, fmt.Sprintf(format, a...))

	if level == fatalLevel {
		os.Exit(1)
	}
}

func (logger *Logger) Debug(format string, a ...interface{}) {
	logger.doPrintf(debugLevel, printDebugLevel, format, a...)
}

func (logger *Logger) Release(format string, a ...interface{}) {
	logger.doPrintf(releaseLevel, printReleaseLevel, format, a...)
}

func (logger *Logger) Error(format string, a ...interface{}) {
	logger.doPrintf(errorLevel, printErrorLevel, format, a...)
}

func (logger *Logger) Fatal(format string, a ...interface{}) {
	logger.doPrintf(fatalLevel, printFatalLevel, format, a...)
}

var gLogger, _ = New("debug", "", log.LstdFlags)

// It's dangerous to call the method on logging
func Export(logger *Logger) {
	if logger != nil {
		gLogger = logger
	}
}

func Debug(format string, a ...interface{}) {
	gLogger.doPrintf(debugLevel, printDebugLevel, format, a...)
}

func Release(format string, a ...interface{}) {
	gLogger.doPrintf(releaseLevel, printReleaseLevel, format, a...)
}

func Error(format string, a ...interface{}) {
	gLogger.doPrintf(errorLevel, printErrorLevel, format, a...)
}

func Fatal(format string, a ...interface{}) {
	gLogger.doPrintf(fatalLevel, printFatalLevel, format, a...)
}

func Close() {
	gLogger.Close()
}
