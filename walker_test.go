package main

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"
)

type ExampleFile struct {
	Name    string
	Content []byte
}

type FileItem struct {
	Name string
	Hash string
	Size string
}

// 解析 walkerhash 生成的输出文件,并形成一个 map 的结构
func ParseWalkerOutputFile(outputFile string) (m map[string]*FileItem, err error) {

	var (
		contentBytes []byte
		content      string
		lines        []string
	)

	contentBytes, err = ioutil.ReadFile(outputFile)
	if err != nil {
		return
	}

	content = string(contentBytes)
	lines = strings.Split(content, "\n")
	m = make(map[string]*FileItem)

	//log.Println("lines: \n", len(lines), "\n", lines, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")

		if len(parts) != 3 {
			log.Println(line, len(parts))
			log.Println(parts)
			err = errors.New("格式错误")
			return
		}

		fi := FileItem{
			Name: parts[0],
			Hash: parts[1],
			Size: parts[2],
		}

		m[fi.Hash] = &fi
	}
	return
}

func TestWalker(t *testing.T) {

	rand.Seed(time.Now().Unix())

	var (
		fileItemMap map[string]*FileItem
	)

	dir, err := ioutil.TempDir("", "walkertest")
	if err != nil {
		t.Error("无法创建临时目录", err)
	}

	nums := 5 // 生成5个文件

	var f *os.File

	exampleFiles := []*ExampleFile{}

	// 生成若干文件,并向文件输出内容
	for i := 0; i < nums; i++ {
		f, err = ioutil.TempFile(dir, "")
		if err != nil {
			t.Error("无法创建文件")
		}

		ef := ExampleFile{
			Name:    f.Name(),
			Content: []byte(fmt.Sprintf("hello%d", rand.Int())),
		}

		f.Write(ef.Content)
		exampleFiles = append(exampleFiles, &ef)

		f.Close()
	}

	outputFile := path.Join(dir, "output")
	def := TaskDef{
		Excludes: nil,
		Root:     dir,
		Output:   outputFile,
	}

	// 调用 do 计算生成的临时目录中的文件
	do(&def)

	// 解析生成的输出文件
	fileItemMap, err = ParseWalkerOutputFile(outputFile)
	if err != nil {
		t.Error(err)
		return
	}


	// 计算 hash值, 并与 do 函数生成的结果进行比较, 全部一致则程序正确
	for _, ef := range exampleFiles {
		hash := fmt.Sprintf("%x", sha1.Sum([]byte(ef.Content)))

		item, ok := fileItemMap[hash]
		if !ok {
			t.Errorf("错误,输出文件中未找到指定的 hash 值, 文件名:%s, hash:%s", ef.Name, hash)
		}

		if item != nil {
			if item.Size != strconv.Itoa(len(ef.Content)) {
				t.Errorf("错误, 文件大小不一致, 文件名: %s", ef.Name)
			}
		}
	}

}
