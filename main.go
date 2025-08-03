package main

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

const PORT = ":8080"

// TODO: Poner esto en un archivo externo y leerlo en runtime
var IGNORE_PATHS = []string{
	"node_modules",
	"go/pkg",
	".git",
}

var MEDIA_ROOT string

type BreadcrumbItem struct {
	Label  string
	Url    string
	IsLast bool
}

type MyDirEntry struct {
	Name        string
	IsDir       bool
	DownloadUrl string
	RelativeUrl string
	ModDate     string
	Size        string
	RawModDate  time.Time // for sorting
}

type PageHomeData struct {
	Title       string
	CurrentUrl  string
	ParentUrl   string
	Files       []MyDirEntry
	Breadcrumbs []BreadcrumbItem
}

func HumanizeBytes(bytes int64) string {
	const (
		// 1 << x = 2^x
		KB = 1 << 10
		MB = 1 << 20
		GB = 1 << 30
		TB = 1 << 40
	)

	float := float64(bytes)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.1fTB", float/float64(TB))
	case bytes >= GB:
		return fmt.Sprintf("%.1fGB", float/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1fMB", float/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.0fKB", float/float64(KB))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

func shouldIgnorePath(path string) bool {
	for _, ignore := range IGNORE_PATHS {
		if strings.Contains(path, "/"+ignore+"/") ||
			strings.HasPrefix(path, ignore+"/") ||
			strings.HasSuffix(path, "/"+ignore) ||
			path == ignore {
			return true
		}
	}
	return false
}

const base64Prefix = "b64"
const base64PrefixSeparator = ":"

var base64PrefixWithSeparator = strings.Join([]string{base64Prefix, base64PrefixSeparator}, "")

func encodeBase64WithPrefix(input string) string {
	encoded := base64.URLEncoding.EncodeToString([]byte(input))
	return strings.Join([]string{base64Prefix, encoded}, base64PrefixSeparator)
}

func decodeBase64WithPrefix(input string) (string, error) {
	encoded := strings.TrimPrefix(input, base64PrefixWithSeparator)
	output, err := base64.URLEncoding.DecodeString(encoded)

	if err != nil {
		return "", err
	}

	return string(output), nil
}

func hasBase64Prefix(input string) bool {
	return strings.HasPrefix(input, base64PrefixWithSeparator)
}

type MainHandler struct{}
type MediaHandler struct{}
type DownloadHandler struct{}

func (MainHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Main handler hit with path: %s\n", r.URL.Path)
}

func (DownloadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fsPath, err := url.QueryUnescape(r.URL.Query().Get("path"))

	if err != nil {
		fmt.Fprintf(w, "Error intentando descargar el archivo: %s", err.Error())
	}

	if len(fsPath) == 0 {
		fmt.Fprint(w, "Nada que descargar")
		return
	}

	lastSlashIdx := strings.LastIndex(fsPath, "/")
	fileName := fsPath[lastSlashIdx+1:]

	if hasBase64Prefix(fileName) {
		fileName, err = decodeBase64WithPrefix(fileName)

		if err != nil {
			fmt.Fprintf(w, "Hubo un error procesando el nombre del archivo: %s\n", err.Error())
		}
	}

	fsPath = path.Join(fsPath[:lastSlashIdx], fileName)

	fileInfo, err := os.Lstat(fsPath)

	if err != nil {
		if os.IsNotExist(err) {
			// TODO: 404
			fmt.Fprint(w, "No hay nada aqui")
			return
		}
		log.Fatal(err)
	}

	if !fileInfo.IsDir() {
		http.ServeFile(w, r, fsPath)
		return
	}

	buf := new(bytes.Buffer)
	wzip := zip.NewWriter(buf)
	err = wzip.AddFS(os.DirFS(fsPath))

	if err != nil {
		fmt.Fprintf(w, "Error al crear el archivo zip: %s", err.Error())
		return
	}

	err = wzip.Close()

	if err != nil {
		fmt.Fprintf(w, "Error al cerrar el archivo zip: %s", err.Error())
		return
	}

	w.Write(buf.Bytes())
}

func GetFiles(root string) ([]MyDirEntry, error) {
	fsPath := path.Join(MEDIA_ROOT, root)
	osFiles, err := os.ReadDir(fsPath)

	if err != nil {
		return nil, err
	}

	var files []MyDirEntry

	for _, file := range osFiles {
		name := file.Name()

		if shouldIgnorePath(name) {
			continue
		}

		info, err := file.Info()

		if err != nil {
			fmt.Println(err.Error())
			continue
		}

		var nameInUrl string
		if file.IsDir() {
			nameInUrl = name
		} else {
			nameInUrl = encodeBase64WithPrefix(name)
		}

		files = append(files, MyDirEntry{
			Name:        name,
			IsDir:       file.IsDir(),
			ModDate:     info.ModTime().Local().Format("02/01/06 15:04"),
			RawModDate:  info.ModTime().Local(),
			Size:        HumanizeBytes(info.Size()),
			RelativeUrl: path.Join("/media", root, nameInUrl),
			DownloadUrl: "/download?path=" + url.QueryEscape(path.Join(fsPath, nameInUrl)),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].RawModDate.After(files[j].RawModDate)
	})

	return files, nil
}

func FindFiles(root string, search string) ([]MyDirEntry, error) {
	fsPath := path.Join(MEDIA_ROOT, root)
	fsDir := os.DirFS(fsPath)

	var files []MyDirEntry

	err := fs.WalkDir(fsDir, ".", func(filePath string, file fs.DirEntry, err error) error {
		if !file.IsDir() && strings.Contains(strings.ToLower(file.Name()), strings.ToLower(search)) {
			fsFilePath := path.Join(fsPath, filePath)

			if shouldIgnorePath(fsFilePath) {
				return nil
			}

			info, err := os.Stat(fsFilePath)

			if err != nil {
				return err
			}

			fileName := file.Name()
			b64Name := encodeBase64WithPrefix(fileName)

			// cannot do replace in case the name is also in the path
			fsFilePath = path.Join(strings.TrimSuffix(fsFilePath, fileName), b64Name)
			filePath = path.Join(strings.TrimSuffix(filePath, fileName), b64Name)

			files = append(files, MyDirEntry{
				Name:        fileName,
				IsDir:       false,
				ModDate:     info.ModTime().Local().Format("02/01/06 15:04"),
				RawModDate:  info.ModTime().Local(),
				Size:        HumanizeBytes(info.Size()),
				RelativeUrl: filePath,
				DownloadUrl: "/download?path=" + url.QueryEscape(fsFilePath),
			})
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].RawModDate.After(files[j].RawModDate)
	})

	return files, nil
}

func (h MediaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Media handler hit with path: %s\n", r.URL.Path)

	handleError := func(err error) {
		fmt.Printf("Error in MediaHandler: %s\n", err.Error())
		errIsNotExist := os.IsNotExist(err)

		if errIsNotExist {
			// TODO: ir a pagina 404
			fmt.Fprintln(w, "No hay nada aqui")
			return
		}

		// TODO: ir a pagina error
		log.Fatal(err)
	}

	relPath := strings.TrimPrefix(r.URL.Path, "/media/")
	lastSlashIdx := strings.LastIndex(relPath, "/")

	var fileName string

	if lastSlashIdx == -1 {
		fileName = relPath
	} else {
		fileName = relPath[lastSlashIdx+1:]
	}

	if hasBase64Prefix(fileName) {
		var err error
		fileName, err = decodeBase64WithPrefix(fileName)

		if err != nil {
			fmt.Fprintf(w, "Error decodificando nombre de archivo %s: %s\n", fileName, err.Error())
			return
		}

		if lastSlashIdx == -1 {
			relPath = fileName
		} else {
			relPath = path.Join(relPath[:lastSlashIdx], fileName)
		}
	}

	fsPath := path.Join(MEDIA_ROOT, relPath)
	fileInfo, err := os.Lstat(fsPath)

	if err != nil {
		fmt.Fprintf(w, "Hubo un error procesando el nombre del archivo: %s\n", err.Error())
	}

	if err != nil {
		handleError(err)
		return
	}

	if !fileInfo.IsDir() {
		http.ServeFile(w, r, fsPath)
		return
	}

	searchParam := r.FormValue("search")

	var files []MyDirEntry
	var filesError error

	if len(searchParam) > 0 {
		files, filesError = FindFiles(relPath, searchParam)
	} else {
		files, filesError = GetFiles(relPath)
	}

	if filesError != nil {
		handleError(filesError)
		return
	}

	var parentDirPath string

	if !strings.HasSuffix(fsPath, MEDIA_ROOT) {
		// Url no termina en MEDIA_ROOT
		s := strings.Split(r.URL.Path, "/")
		// Quitamos media-query y el ultimo elemento, y unimos para crear el parent path
		parentDirPath = strings.Join(s[1:len(s)-1], "/")
	}

	var currentBaseUrl strings.Builder
	var breadcrumbs []BreadcrumbItem

	currentBaseUrl.WriteString("/")
	urlParts := strings.Split(r.URL.Path, "/")

	// quita primer elemento por que "/media" = "['', 'media']"
	urlParts = urlParts[1:]

	// quita ultimo elemento para cuando ruta termina en "/". En ese caso "media/" = "['media', '']"
	if len(urlParts[len(urlParts)-1]) == 0 {
		urlParts = urlParts[:len(urlParts)-1]
	}

	for i, part := range urlParts {
		currentBaseUrl.WriteString(part)
		currentBaseUrl.WriteString("/")

		breadcrumbs = append(breadcrumbs, BreadcrumbItem{
			Label:  part,
			Url:    currentBaseUrl.String(),
			IsLast: i == len(urlParts)-1,
		})
	}

	currentUrl := strings.TrimSuffix(r.URL.Path, "/")

	tmpl := template.Must(template.ParseFiles("templates/base.tmpl", "templates/directory.tmpl"))
	data := PageHomeData{
		Title:       fmt.Sprintf("Yogusita - %s", currentUrl),
		CurrentUrl:  currentUrl,
		ParentUrl:   parentDirPath,
		Files:       files,
		Breadcrumbs: breadcrumbs,
	}

	if err := tmpl.ExecuteTemplate(w, "base.tmpl", data); err != nil {
		log.Fatalln(err)
	}
}

func init() {
	MEDIA_ROOT = os.Getenv("MEDIA_ROOT")

	if v := os.Getenv("MEDIA_ROOT"); v != "" {
		MEDIA_ROOT = v
	} else {
		log.Fatal("Env var MEDIA_ROOT not found")
	}
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "a")
	})

	mux.Handle("/", MainHandler{})
	mux.Handle("/media/", MediaHandler{})
	mux.Handle("/download/", DownloadHandler{})
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	fmt.Printf("Listening on port %s\n", PORT)

	log.Panic(http.ListenAndServe(PORT, mux))
}
