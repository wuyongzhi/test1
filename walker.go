package main

import "os"

type FileHandler func(f *os.File)

type TaskDef struct {
	Excludes []string
	Root     string
	Output   string
}

func walker(handler FileHandler) {

}

func printHelp() {

}

func parseCommandLine() {

}

func do (def *TaskDef) {

}

func main() {

}
