// go log project log.go
package mylog

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log/syslog"
	"os"
	//"os/signal"
	"runtime"
	"strconv"
	"sync"
	//"syscall"
	"time"
)

type SYSLOGConfig struct {
	Use     bool   `json:"Use"`
	Type    string `json:"Type"`
	Address string `json:"Address"`
	Port    int    `json:"Port"`
}

type MYLogConfig struct {
	Dir          string       `json:"LOGDIR"`
	MaxFileSize  int64        `json:"MAXFILESIZE"`
	MaxFileCount int          `json:"MAXFILECOUNT"`
	Level        int          `json:"LEVEL"`
	Module       string       `json:"MODULE"`
	Console      bool         `json:"CONSOLE"`
	Syslog       SYSLOGConfig `json:"SYSLOG"`
	Log2File	bool `json:"LOG2FILE"`
}

const (
	FATAL int = iota
	ERROR
	WARN
	INFO
	DEBUG
)

var (
	logConfig *MYLogConfig = new(MYLogConfig)
	mu        *sync.RWMutex
	index     int  = 1
	first     bool = true
	logInit   bool = false
	txid      string
)

func loadLogConfig(fileName string) (e error) {
	f, err := ioutil.ReadFile(fileName)
	if err != nil {
		fmt.Println("无法读取日志配置文件")
		return err
	}

	err = json.Unmarshal(f, &logConfig)
	if err != nil {
		fmt.Println("无法分析日志配置文件")
		return err
	}

	//对不合法的参数重置为默认值
	if logConfig.Dir == "" {
		logConfig.Dir = "." + string(os.PathSeparator) + "log"
	}
	if DEBUG < logConfig.Level || FATAL > logConfig.Level {
		logConfig.Level = INFO
	}
	if logConfig.MaxFileCount > 50 || logConfig.MaxFileCount < 1 {
		logConfig.MaxFileCount = 15
	}
	if logConfig.MaxFileSize < 0 || logConfig.MaxFileSize > (4096*1000*1000) {
		logConfig.MaxFileSize = 40960
	}
	if logConfig.Module == "" {
		logConfig.Module = "DEFAULT"
	}
	//if logConfig.Console == nil {
	logConfig.Console = true
	//}
	//if logConfig.Console == nil {
	logConfig.Log2File = false
	//}

	return nil
}

func InitLog(fileName string, id string) {
	txid = id

	//加载配置
	err := loadLogConfig(fileName)
	if err != nil {
		fmt.Printf("加载日志文件[%s] 异常 [%s].\n", fileName, err)
		//os.Exit(99)
		//使用默认日志参数
		fmt.Println("使用默认参数")
		logConfig.Dir = "." + string(os.PathSeparator) + "log"
		logConfig.Level = INFO
		logConfig.MaxFileCount = 15
		logConfig.MaxFileSize = 40960
		logConfig.Module = "DEFAULT"
		logConfig.Console = true
		logConfig.Syslog.Use = true
		logConfig.Syslog.Address = "172.17.0.1"
		logConfig.Syslog.Port = 514
		logConfig.Syslog.Type = "udp"
		logConfig.Log2File = false
	}
	fmt.Printf("日志参数信息 DIR[%s] MAXFILECOUNT[%d] MAXFILESIZE[%d] LEVEL[%d] MODULE[%s] CONSOLE[%s].\n", logConfig.Dir, logConfig.MaxFileCount, logConfig.MaxFileSize, logConfig.Level, logConfig.Module, logConfig.Console)

	mu = new(sync.RWMutex)

	//设置通知，通过kill -SIGUSR2 pid来触发配置热加载
/*
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGUSR2)
	go func() {
		for {
			<-s
			loadLogConfig(fileName)
			var message = fmt.Sprintf("日志参数信息 DIR[%s] MAXFILECOUNT[%d] MAXFILESIZE[%d] LEVEL[%d] MODULE[%s] CONSOLE[%s].\n", logConfig.Dir, logConfig.MaxFileCount, logConfig.MaxFileSize, logConfig.Level, logConfig.Module, logConfig.Console)
			Warn("reload config")
			Warn(message)
		}
	}()
*/
	logInit = true
}

func myLog(m string) {

	//实际日志信息
	var now = time.Now()
	var logTime = now.Format("2006-01-02 15:04:05.999")
	var message = fmt.Sprintf("%s %s\n", logTime, m)

	//输出到屏幕
	if logConfig.Console {
		fmt.Print(message)
	}

	//输出到syslog
	if logConfig.Syslog.Use {
		host := fmt.Sprintf("%s:%d", logConfig.Syslog.Address, logConfig.Syslog.Port)
		sysLog, sysErr := syslog.Dial(logConfig.Syslog.Type, host, syslog.LOG_WARNING|syslog.LOG_LOCAL0, txid)
		defer sysLog.Close()
		if sysErr == nil {
			sysLog.Write([]byte(m))
		}
	}
	//输出到文件
	if ! logConfig.Log2File {
		return
	}

	mu.Lock()
	defer mu.Unlock()

	//建立日志目录
	_, err := os.Stat(logConfig.Dir)
	b := err == nil || os.IsExist(err)
	if !b {
		if err1 := os.MkdirAll(logConfig.Dir, 0666); err1 != nil {
			if os.IsPermission(err1) {
				fmt.Printf("无法创建日志目录[%s]，错误信息[%s]", logConfig.Dir, err1.Error())
				return
			}
		}
	}

	//打开实际文件
	var logDate = now.Format("20060102")
	var logObj *os.File
	var fileName = logConfig.Dir + string(os.PathSeparator) + logConfig.Module + "_" + logDate + "_" + strconv.Itoa(index) + ".log"
	s, err1 := os.Stat(fileName)
	if err1 != nil && os.IsNotExist(err1) {
		//file not exist
		logObj, _ = os.OpenFile(fileName, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0666)
	} else {
		//"file exist"
		if first {
			e := os.Remove(fileName)
			if e != nil {
				fmt.Println(e)
			}
			//fmt.Println("come in first time, remove:", fileName)
			logObj, _ = os.OpenFile(fileName, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0666)
			first = false
		} else {
			if s.Size() >= logConfig.MaxFileSize {
				//文件超长后重置信息
				index += 1
				if index > logConfig.MaxFileCount {
					index = 1
				}
				fileName = logConfig.Dir + string(os.PathSeparator) + logConfig.Module + "_" + logDate + "_" + strconv.Itoa(index) + ".log"
				_, err2 := os.Stat(fileName)
				if err2 == nil || os.IsExist(err2) {
					//fmt.Println("file extend and removd:", fileName)
					e2 := os.Remove(fileName)
					if e2 != nil {
						fmt.Println(e2)
					}
				}
				//fmt.Println("come size extend:", fileName)
				logObj, _ = os.OpenFile(fileName, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0666)
			} else {
				//fmt.Println("come normal size:", fileName)
				logObj, _ = os.OpenFile(fileName, os.O_RDWR|os.O_APPEND, 0666)
			}
		}
	}

	defer logObj.Close()

	//写入实际内容
	logObj.WriteString(message)
	logObj.Sync()
}

func Fatal(format string, m ...interface{}) {
	if !logInit {
		loadLogConfig("./etc/log.json")
	}
	if logConfig.Level < FATAL {
		return
	}
	var message = fmt.Sprintf(format, m...)
	//pc, file, line, ok := runtime.Caller(0)
	pc, _, _, _ := runtime.Caller(1)
	message = "[FATAL] [" + runtime.FuncForPC(pc).Name() + "]" + message
	myLog(message)
}

func Error(format string, m ...interface{}) {
	if !logInit {
		loadLogConfig("./etc/log.json")
	}
	if logConfig.Level < ERROR {
		return
	}
	var message = fmt.Sprintf(format, m...)
	//pc, file, line, ok := runtime.Caller(0)
	pc, _, _, _ := runtime.Caller(1)
	message = "[ERROR] [" + runtime.FuncForPC(pc).Name() + "]" + message
	myLog(message)
}

func Warn(format string, m ...interface{}) {
	if !logInit {
		loadLogConfig("./etc/log.json")
	}
	if logConfig.Level < WARN {
		return
	}
	var message = fmt.Sprintf(format, m...)
	//pc, file, line, ok := runtime.Caller(0)
	pc, _, _, _ := runtime.Caller(1)
	message = "[WARN] [" + runtime.FuncForPC(pc).Name() + "]" + message
	myLog(message)
}

func Info(format string, m ...interface{}) {
	if !logInit {
		loadLogConfig("./etc/log.json")
	}
	if logConfig.Level < INFO {
		return
	}
	var message = fmt.Sprintf(format, m...)
	//pc, file, line, ok := runtime.Caller(1)
	pc, _, _, _ := runtime.Caller(1)
	message = "[INFO] [" + runtime.FuncForPC(pc).Name() + "]" + message
	myLog(message)
}

func Debug(format string, m ...interface{}) {
	if !logInit {
		loadLogConfig("./etc/log.json")
	}
	if logConfig.Level < DEBUG {
		return
	}
	var message = fmt.Sprintf(format, m...)
	//pc, file, line, ok := runtime.Caller(0)
	pc, _, _, _ := runtime.Caller(1)
	message = "[DEBUG] [" + runtime.FuncForPC(pc).Name() + "]" + message
	myLog(message)
}
