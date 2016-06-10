package main

import (
	"runtime"
	"crypto/sha1"
	"hash"
	"io"
	"os"
	"fmt"
	"sync"
	"path/filepath"
	"sync/atomic"
	"log"
)

func printHelp() {

}

func parseCommandLine() {

}



type FileHandler func(filePath string)

type TaskDef struct {
	Excludes []string
	Root     string
	Output   string

	readCount int
}

type WorkItem struct {
	Error   error
	Name string
	FilePath string
	Size int64
	Hash []byte
}

//
//	计算文件hash
//
func compute(item *WorkItem, h hash.Hash, )  {

	f, err := os.Open(item.FilePath)
	if err != nil {
		item.Error = err
		return
	}
	defer func () {
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
	defer func () {
		outputFile.Close()
	}()



	computeWaitGroup := sync.WaitGroup{}


	readFileChannel := make(chan *WorkItem, numCpu * 5)
	outputChannel := make(chan *WorkItem, 64)


	// 有几个CPU核心,就启动几个 goroutine 计算
	for i:=0; i<numCpu; i++ {
		atomic.AddInt32(pointerComputeRoutines, 1)
		computeWaitGroup.Add(1)
		go func() {
			sha1Hash := sha1.New()

			for {
				item, ok := <- readFileChannel
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
	go func () {
		for {
			item, ok:= <- outputChannel

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
	err = filepath.Walk(def.Root, func (path string, info os.FileInfo, err error) error {

		if info.IsDir() {
			return nil
		}
		// TODO  排除忽略的文件

		item := WorkItem {
			Error: err,
			FilePath: path,
			Size: info.Size(),
		}

		readFileChannel <- &item

		return nil
	})


	close(readFileChannel)

	computeWaitGroup.Wait()

}

func main() {

	// TODO 处理命令行参数

	def := TaskDef{
		Excludes: []string{"*.txt"},
		Root:     "/Users/wuyongzhi/IdeaProjects/test1/exampleDir",
		Output:   "walkerOutput.txt",
	}


	do(&def)

}
