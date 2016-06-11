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

func printHelp() {
	helperText := `功能:
	遍历文件夹并计算其中文件的 SHA1 哈希值, 将结果输出到指定文件中

用法:
	遍历当前目录:
		hashwalker

	遍历指定目录:
		hashwalker 目录名

	遍历指定目录,并排除特定文件(支持通配符 *, ?):
		hashwalker 目录名 --exclude *.txt [模式2,模式3...]

	打印此帮助信息

		hashwalker --help | -? | ?

`

	//log.Println("遍历文件夹并计算其中文件的 SHA1 哈希值, 将结果输出到指定文件中")
	//log.Println()
	//log.Println("\t用法: ")
	//log.Println()
	//
	//log.Println("遍历当前目录")
	//log.Println("\t\thashwalker")
	//log.Println()
	//
	//log.Println("遍历指定目录")
	//log.Println("\t\thashwalker 目录名 ")
	//
	//
	//log.Println("\t\thashwalker 目录名 --exclude 排除模式1 [排除模式2...] ")

	fmt.Print(helperText)
}

func parseCommandLine() *TaskDef {

	excludesValue := flag.String("excludes", "", "忽略指定的文件,多个通配符之间用空格分开,并加双引号. 如: -excludes=\"*.jpg *.gif *.png\"")
	rootValue := flag.String("root", "", "指定根目录,如果不指定,默认使用当前目录")
	output := flag.String("output", "", "指定输出文件名,如果不指定,默认使用 hashwalker_output.txt")
	r := regexp.MustCompile("\\s")
	excludes := r.Split(*excludesValue, -1)

	flag.Parse()

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

type FileHandler func(filePath string)

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

func do(def *TaskDef) {

	numCpu := runtime.NumCPU()

	var computeRoutines int32 = 0
	var pointerComputeRoutines = &computeRoutines

	outputFile, err := os.Create(def.Output)
	if err != nil {
		panic(err)
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

		if err != nil || info.IsDir() {
			return nil
		}

		// TODO  处理跳过忽略的文件

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

	// TODO 处理命令行参数

	def := parseCommandLine()

	if def == nil {
		return
	}

	do(def)
}
