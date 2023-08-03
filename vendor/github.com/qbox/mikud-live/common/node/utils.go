package cnode

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"strings"
)

func ReadLines(filename string) ([]string, error) {
	return ReadLinesOffsetN(filename, 0, -1)
}

func ReadLinesOffsetN(filename string, offset uint, n int) (ret []string, err error) {
	f, err := os.Open(filename)
	if err != nil {
		return []string{""}, err
	}
	defer f.Close()

	r := bufio.NewReader(f)
	for i := 0; i < int(offset)+n || n < 0; i++ {
		line, err := r.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				ret = append(ret, strings.Trim(line, "\r\n"))
			}
			break
		}
		if i < int(offset) {
			continue
		}
		ret = append(ret, strings.Trim(line, "\r\n"))
	}
	return
}

func ToJson(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
