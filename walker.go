package main

import (
	"crypto/sha1"
	"flag"
	"fmt"
	"hash"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"sync/atomic"
)

func parseCommandLine() *TaskDef {

	excludesValue := flag.String("excludes", "", "忽略指定的文件,多个通配符之间用空格分开,并加双引号. 如: -excludes=\"*.jpg *.gif *.png\"")
	rootValue := flag.String("root", "", "指定根目录,如果不指定,默认使用当前目录")
	output := flag.String("output", "", "指定输出文件名,如果不指定,默认使用 hashwalker_output.txt")

	flag.Parse()

	r := regexp.MustCompile("\\s")
	excludes := r.Split(*excludesValue, -1)

	def := TaskDef{
		Excludes: excludes,
		Root:     *rootValue,
		Output:   *output,
	}

	if def.Root == "" {
		def.Root, _ = os.Getwd()
	}
	if def.Output == "" {
		def.Output = "hashwalker_output.txt"
	}

	return &def
}

type TaskDef struct {
	Excludes []string
	Root     string
	Output   string

	readCount int
}

type WorkItem struct {
	Error    error
	Name     string
	FilePath string
	Size     int64
	Hash     []byte
}

//
//	计算文件hash
//
func compute(item *WorkItem, h hash.Hash) {

	f, err := os.Open(item.FilePath)
	if err != nil {
		item.Error = err
		return
	}
	defer func() {
		f.Close()
	}()

	h.Reset()
	io.Copy(h, f)
	item.Hash = h.Sum(nil)
}

func (me *TaskDef) IsExclude(path string) bool {
	base := filepath.Base(path)
	for _, exclude := range me.Excludes {
		if ok, _ := filepath.Match(exclude, base); ok {
			return true
		}
	}
	return false
}


func do(def *TaskDef) {

	numCpu := runtime.NumCPU()

	var computeRoutines int32 = 0
	var pointerComputeRoutines = &computeRoutines

	outputFile, err := os.Create(def.Output)
	if err != nil {
		log.Panic("创建输出文件错误:", err)
	}
	defer func() {
		outputFile.Close()
	}()

	computeWaitGroup := sync.WaitGroup{}

	readFileChannel := make(chan *WorkItem, numCpu*5)
	outputChannel := make(chan *WorkItem, 64)

	// 有几个CPU核心,就启动几个 goroutine 计算
	for i := 0; i < numCpu; i++ {
		atomic.AddInt32(pointerComputeRoutines, 1)
		computeWaitGroup.Add(1)
		go func() {
			sha1Hash := sha1.New()

			for {
				item, ok := <-readFileChannel
				if ok {
					compute(item, sha1Hash)
					outputChannel <- item
				} else {
					break
				}
			}

			computeWaitGroup.Done()

			if 0 == atomic.AddInt32(pointerComputeRoutines, -1) {
				close(outputChannel)
			}
		}()
	}

	log.Printf("Started %d goroutines.", computeRoutines)

	// 启动输出结果 goroutine
	computeWaitGroup.Add(1)
	go func() {
		for {
			item, ok := <-outputChannel

			if ok {
				if item.Error == nil {
					io.WriteString(outputFile, fmt.Sprintf("%s,%x,%d\n", item.FilePath, item.Hash, item.Size))
				}
			} else {
				break
			}
		}

		computeWaitGroup.Done()
	}()



	log.Println(def.Excludes)

	// 遍历文件
	err = filepath.Walk(def.Root, func(path string, info os.FileInfo, err error) error {

		if err != nil {
			return nil
		}

		if info.IsDir() {
			if def.IsExclude(path) {
				return filepath.SkipDir
			}
		} else if def.IsExclude(path) {
			return nil
		}

		item := WorkItem{
			FilePath: path,
			Size:     info.Size(),
		}

		readFileChannel <- &item

		return nil
	})

	if err != nil {
		log.Printf("Walk Dir '%s' error: %s", def.Root, err.Error())
	}

	close(readFileChannel)

	computeWaitGroup.Wait()

}

func main() {

	def := parseCommandLine()

	if def == nil {
		return
	}

	do(def)
}
