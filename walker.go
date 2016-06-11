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
	"time"
)

// 代表一次任务处理的定义, 包含扫描的根目录, 输出文件名, 排除的文件模式定义
type TaskDef struct {
	Excludes []string
	Root     string
	Output   string
}

// 一个小的帮助函数,检查一个指定的路径是否在排除范围内
func (me *TaskDef) IsExclude(path string) bool {
	base := filepath.Base(path)
	for _, exclude := range me.Excludes {
		if ok, _ := filepath.Match(exclude, base); ok {
			return true
		}
	}
	return false
}

// 代表一个文件处理的输入输出项. 输入文件路径,处理结果为哈希值, 文件大小等
type WorkItem struct {
	Error    error
	Name     string
	FilePath string
	Size     int64
	Hash     []byte
}

// 解析命令行参数,并返回一个 TaskDef 结构. 处理一些默认值的情况
func parseCommandLine() *TaskDef {

	excludesValue := flag.String("excludes", "", "忽略指定的文件,多个通配符之间用空格分开,并加双引号. 如: -excludes=\"*.jpg *.gif *.png\"")
	rootValue := flag.String("root", "", "指定根目录,如果不指定,默认使用当前目录")
	output := flag.String("output", "hashwalker_output.txt", "指定输出文件名,如果不指定,默认使用 hashwalker_output.txt")

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

	return &def
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

//
// 程序核心开始执行的地方, 接受一个 TaskDef 结构, 并依据它来开始处理.
// 大致逻辑:
// 1. 创建两条数据管道, 一条用于放置待处理的文件, 一条用于放置处理后的文件.
// 2. 待处理的文件管道, 输入端由 1 个 goroutine 来处理, 遍历指定目录, 并将符合条件的文件扔进管道中,
// 		输出端由一批 goroutine来同时处理, goroutine 数量由当前CPU核数来确定. 它们负责从管道中取出待处理文件,并计算HASH值, 然后扔到
//		处理后的文件管道中
// 3. 处理后的文件管道, 输入端上面说了, 输出端由 1 个 goroutine 来处理, 从管道中取出处理完的结果, 写入到输出文件中
//
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

	// 遍历文件
	err = filepath.Walk(def.Root, func(path string, info os.FileInfo, err error) error {

		if err != nil {
			return nil
		}

		// 忽略指定目录或文件
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

	now := time.Now()
	do(def)
	end := time.Now()

	log.Println("总共耗时:", end.Sub(now))
}
