package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
)

const PORT = ":8080"
const MEDIA_ROOT = "/home/yogusita/"

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
}

type PageHomeData struct {
	Title         string
	Heading       string
	CurrentFsPath string
	CurrentUrl    string
	ParentUrl     string
	Files         []MyDirEntry
	Breadcrumbs   []BreadcrumbItem
}

func HumanizeBytes(bytes int64) string {
	const (
		// 1 << X = 2^x
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

	fmt.Printf("Fs Path: %s\n", fsPath)

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

func (h MediaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Media handler hit with path: %s\n", r.URL.Path)

	// necesito que si es /media/mi/carpeta
	// localpath es MEDIA_ROOT/mi/carpeta
	fsPath := strings.Replace(r.URL.Path, "/media/", MEDIA_ROOT, 1)

	var parentDirPath string

	if !strings.HasSuffix(fsPath, MEDIA_ROOT) {
		// Url no termina en MEDIA_ROOT
		s := strings.Split(r.URL.Path, "/")
		// Quitamos media-query y el ultimo elemento, y unimos para crear el parent path
		parentDirPath = strings.Join(s[1:len(s)-1], "/")
	}

	fmt.Printf("> parentDirPath : %s\n", parentDirPath)

	handleOsError := func(err error) {
		fmt.Printf("> Error reading path: %s\n", fsPath)
		fmt.Printf("> Error:%s\n", err)
		errIsNotExist := os.IsNotExist(err)

		if errIsNotExist {
			// TODO: ir a pagina 404
			fmt.Fprintln(w, "No hay nada aqui")
			return
		}

		// TODO: ir a pagina error
		log.Fatal(err)
	}

	fileInfo, err := os.Lstat(fsPath)

	if err != nil {
		handleOsError(err)
		return
	}

	// TODO: handle symlinks?
	if fileInfo.IsDir() {
		osFiles, err := os.ReadDir(fsPath)

		files := make([]MyDirEntry, len(osFiles))

		for i, file := range osFiles {
			name := file.Name()

			info, err := file.Info()

			if err != nil {
				// TODO: decidir que hacer aqui
				fmt.Println(err.Error())
				continue
			}

			files[i] = MyDirEntry{
				Name:  name,
				IsDir: file.IsDir(),
				// Format uses numbers to identify the format: 02=day, 01=month, 15=hour, etc.
				ModDate:     info.ModTime().Local().Format("02/01/06 15:04"),
				Size:        HumanizeBytes(info.Size()),
				RelativeUrl: path.Join(r.URL.Path, name),
				DownloadUrl: "/download?path=" + url.QueryEscape(path.Join(fsPath, name)),
			}
		}

		if err != nil {
			handleOsError(err)
			return
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

		tmpl := template.Must(template.ParseFiles("templates/base.tmpl", "templates/directory.tmpl"))
		data := PageHomeData{
			Title:         "Yogusita Media Server",
			Heading:       "Bienvenid@ a Yogusita Media Server",
			CurrentFsPath: strings.TrimSuffix(fsPath, "/"),
			CurrentUrl:    strings.TrimSuffix(r.URL.Path, "/"),
			ParentUrl:     parentDirPath,
			Files:         files,
			Breadcrumbs:   breadcrumbs,
		}

		if err := tmpl.ExecuteTemplate(w, "base.tmpl", data); err != nil {
			log.Fatalln(err)
		}

		return
	}

	http.ServeFile(w, r, fsPath)
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
