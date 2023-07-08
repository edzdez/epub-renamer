package main

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/fatih/color"
	"github.com/gabriel-vasile/mimetype"
)

type BookData struct {
	Title  string `xml:"metadata>title"`
	Author string `xml:"metadata>creator"`
}

type EpubMetadataParseError struct{}

func (e *EpubMetadataParseError) Error() string {
	return "failed to find epub opf"
}

func parseContentOPF(rc io.ReadCloser) (BookData, error) {
	byteValue, err := io.ReadAll(rc)
	if err != nil {
		return BookData{}, err
	}

	var bookData BookData
	if err = xml.Unmarshal(byteValue, &bookData); err != nil {
		return BookData{}, err
	}

	return bookData, nil
}

func readEpubData(f *zip.ReadCloser) (BookData, error) {
	for _, file := range f.File {
		if strings.HasSuffix(file.Name, ".opf") {
			rc, err := file.Open()
			if err != nil {
				return BookData{}, err
			}
			defer rc.Close()

			return parseContentOPF(rc)
		}
	}

	return BookData{}, &EpubMetadataParseError{}
}

func sanitizeData(data *BookData) string {
	title := regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(data.Title, "_")
	author := regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(data.Author, "")

	return title + "-" + author + ".epub"
}

func run(file string, outputDirectory string, result chan struct {
	string
	bool
}) {
	mtype, err := mimetype.DetectFile(file)
	if err != nil {
		log.Print(err.Error())
		result <- struct {
			string
			bool
		}{file, false}
		return
	}

	if mtype.String() != "application/epub+zip" {
		log.Print(file + ": not an epub file")
		result <- struct {
			string
			bool
		}{file, false}
		return
	}

	var data BookData
	{
		f, err := zip.OpenReader(file)
		if err != nil {
			log.Print(err.Error())
			result <- struct {
				string
				bool
			}{file, false}
			return
		}
		defer f.Close()

		data, err = readEpubData(f)
		if err != nil {
			log.Print(file + ": " + err.Error())
			result <- struct {
				string
				bool
			}{file, false}
			return
		}
	}

	filename := sanitizeData(&data)
	if filename == "" {
		log.Print("empty output filename... aborting")
		result <- struct {
			string
			bool
		}{file, false}
		return
	}

	fout, err := os.Create(outputDirectory + "/" + filename)
	if err != nil {
		log.Print(err.Error())
		result <- struct {
			string
			bool
		}{file, false}
		return
	}
	defer fout.Close()

	fin, err := os.Open(file)
	if err != nil {
		log.Print(err.Error())
		result <- struct {
			string
			bool
		}{file, false}
		return
	}
	defer fin.Close()

	_, err = io.Copy(fout, fin)
	if err != nil {
		log.Print(err.Error())
		result <- struct {
			string
			bool
		}{file, false}
		return
	}

	result <- struct {
		string
		bool
	}{file, true}
}

func isDirectory(path string) (bool, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false, err
	}

	return fileInfo.IsDir(), nil
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage:", os.Args[0], "<output_directory> <files> ...")
		os.Exit(1)
	}

	outputDirectory := os.Args[1]
	isDir, err := isDirectory(outputDirectory)
	if err != nil {
		log.Print(err.Error())
		os.Exit(1)
	} else if !isDir {
		log.Print(os.Args[1] + " is not a directory!")
		os.Exit(1)
	}

	files := os.Args[2:]
	results := map[string]bool{}
	resultsChan := make(chan struct {
		string
		bool
	})

	for _, file := range files {
		go run(file, outputDirectory, resultsChan)
	}

	for i := 0; i < len(files); i++ {
		result := <-resultsChan
		results[result.string] = result.bool
	}

	succeeded := 0
	failed := 0
	for file, result := range results {
		if result {
			succeeded += 1
			color.Green("%s: ✅", file)
		} else {
			failed += 1
			color.Red("%s: ❌", file)
		}
	}

	fmt.Println("succeeded:", succeeded)
	fmt.Println("failed:", failed)
}
