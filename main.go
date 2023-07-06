package main

import (
	"archive/zip"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/gabriel-vasile/mimetype"
)

type BookData struct {
	Title  string `xml:"metadata>title"`
	Author string `xml:"metadata>creator"`
}

type EpubMetadataParseError struct{}

func (e *EpubMetadataParseError) Error() string {
	return "failed to find content.opf"
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
		if strings.HasSuffix(file.Name, "content.opf") {
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

func run(file string, outputDirectory string, result chan bool) {
	mtype, err := mimetype.DetectFile(file)
	if err != nil {
		log.Print(err.Error())
		result <- false
		return
	}

	if mtype.String() != "application/epub+zip" {
		log.Print(file + ": not an epub file")
		result <- false
		return
	}

	var data BookData
	{
		f, err := zip.OpenReader(file)
		if err != nil {
			log.Print(err.Error())
			result <- false
			return
		}
		defer f.Close()

		data, err = readEpubData(f)
		if err != nil {
			log.Print(err.Error())
			result <- false
			return
		}
	}

	filename := sanitizeData(&data)
	if filename == "" {
		log.Print("empty output filename... aborting")
		result <- false
		return
	}

	fout, err := os.Create(outputDirectory + "/" + filename)
	if err != nil {
		log.Print(err.Error())
		result <- false
		return
	}
	defer fout.Close()

	fin, err := os.Open(file)
	if err != nil {
		log.Print(err.Error())
		result <- false
		return
	}
	defer fin.Close()

	_, err = io.Copy(fout, fin)
	if err != nil {
		log.Print(err.Error())
		result <- false
		return
	}

    result <- true
}

func main() {
	if len(os.Args) < 2 {
		log.Panic("expected file argument")
	}

	outputDirectory := flag.String("output", ".", "output directory for epub-renamer")

	files := os.Args[1:]
	results := map[string]bool{}
	for _, file := range files {
		result := make(chan bool)
		go run(file, *outputDirectory, result)

        value := <- result
		results[file] = value
	}

	for file, result := range results {
        fmt.Println(file, ": ", result)
	}
}
