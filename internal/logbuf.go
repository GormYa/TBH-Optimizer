package internal

import (
	"fmt"
	"regexp"
	"sync"
	"time"
)

type LogEntry struct {
	Seq  int      `json:"seq"`
	Time string   `json:"time"`
	Kind string   `json:"kind"`
	Msg  string   `json:"msg"`
	Key  string   `json:"key"`
	Args []string `json:"args"`
}

var verbRe = regexp.MustCompile(`%[-+# 0-9.*]*[bcdeEfFgGoqstTvxXUp]`)

const logBufMax = 400

var (
	logMu  sync.Mutex
	logBuf []LogEntry
	logSeq int
)

func Logf(kind, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	verbs := verbRe.FindAllString(format, -1)
	rendered := make([]string, 0, len(args))
	for i, a := range args {
		verb := "%v"
		if i < len(verbs) {
			verb = verbs[i]
		}
		rendered = append(rendered, fmt.Sprintf(verb, a))
	}
	logMu.Lock()
	logSeq++
	logBuf = append(logBuf, LogEntry{Seq: logSeq, Time: time.Now().Format("15:04:05"), Kind: kind, Msg: msg, Key: format, Args: rendered})
	if len(logBuf) > logBufMax {
		logBuf = logBuf[len(logBuf)-logBufMax:]
	}
	logMu.Unlock()
	fmt.Println(msg)
}

// LogsSince devolve as linhas com Seq > since (para polling incremental do front).
func LogsSince(since int) []LogEntry {
	logMu.Lock()
	defer logMu.Unlock()
	out := make([]LogEntry, 0, 16)
	for _, e := range logBuf {
		if e.Seq > since {
			out = append(out, e)
		}
	}
	return out
}
