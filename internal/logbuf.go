package internal

import (
	"fmt"
	"sync"
	"time"
)

// LogEntry e uma linha do console do app, consumida pelo front via /api/logs.
type LogEntry struct {
	Seq  int    `json:"seq"`
	Time string `json:"time"`
	Kind string `json:"kind"` // info | run | reject
	Msg  string `json:"msg"`
}

const logBufMax = 400

var (
	logMu  sync.Mutex
	logBuf []LogEntry
	logSeq int
)

// Logf grava uma linha no console interno (e tambem no stdout, util em dev). kind
// colore a linha no front: "run" (verde) corrida registrada, "reject" (vermelho)
// corrida descartada com o motivo, "info" (neutro) eventos gerais.
func Logf(kind, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	logMu.Lock()
	logSeq++
	logBuf = append(logBuf, LogEntry{Seq: logSeq, Time: time.Now().Format("15:04:05"), Kind: kind, Msg: msg})
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
