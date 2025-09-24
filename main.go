package main

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"media-server/config"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

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
	FsRelPath   string
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
	ignorePaths := config.GetConfig().IgnorePaths

	for _, ignore := range ignorePaths {
		if strings.Contains(path, "/"+ignore+"/") ||
			strings.HasPrefix(path, ignore+"/") ||
			strings.HasSuffix(path, "/"+ignore) ||
			path == ignore {
			return true
		}
	}
	return false
}

func encodeBase64WithPrefix(input string) string {
	base64Prefix := config.GetConfig().Base64NamePrefix

	encoded := base64.URLEncoding.EncodeToString([]byte(input))
	return strings.Join([]string{base64Prefix, encoded}, "")
}

func decodeBase64WithPrefix(input string) (string, error) {
	base64Prefix := config.GetConfig().Base64NamePrefix

	encoded := strings.TrimPrefix(input, base64Prefix)
	output, err := base64.URLEncoding.DecodeString(encoded)

	if err != nil {
		return "", err
	}

	return string(output), nil
}

func hasBase64Prefix(input string) bool {
	return strings.HasPrefix(input, config.GetConfig().Base64NamePrefix)
}

/// Returns the absolute media path. Returns an error if the path is invalid, or outside MEDIA_ROOT
func getAbsoluteMediaPath(rawEscapedPath string) (string, error) {
	queryPath, err := url.QueryUnescape(rawEscapedPath)

	if err != nil {
		// TODO: replicate this Error usage in other parts of the server
		return "", err;
	}

	if len(queryPath) == 0 {
		return "", errors.New("Invalid path")
	}

	absMedia, _ := filepath.Abs(MEDIA_ROOT);
	absPath, _ := filepath.Abs(filepath.Join(absMedia, queryPath));

	rel, err := filepath.Rel(absMedia, absPath)

	if err != nil {
		return "", err
	}

	if strings.HasPrefix(rel, "..") {
		return "", errors.New("Invalid path")
	}

	return absPath, nil
}

type MainHandler struct{}
func (MainHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Main handler hit with path: %s\n", r.URL.Path)
}

type DownloadHandler struct{}
func (DownloadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	absPath, err := getAbsoluteMediaPath(r.URL.Query().Get("path"))

	if err != nil {
		// TODO: replicate this Error usage in other parts of the server
		http.Error(w,err.Error(), http.StatusBadRequest);
		return;
	}

	lastSlashIdx := strings.LastIndex(absPath, "/")
	fileName := absPath[lastSlashIdx+1:]

	if hasBase64Prefix(fileName) {
		fileName, err = decodeBase64WithPrefix(fileName)

		if err != nil {
			fmt.Fprintf(w, "Error processing the file name \"%s\": %s\n", fileName, err.Error())
			return;
		}
	}

	absPath = path.Join(absPath[:lastSlashIdx], fileName)

	fileInfo, err := os.Lstat(absPath)

	if err != nil {
		if os.IsNotExist(err) {
			// TODO: 404
			fmt.Fprint(w, "Nothing here")
			return
		}
		log.Fatal(err)
	}

	if !fileInfo.IsDir() {
		http.ServeFile(w, r, absPath)
		return
	}

	buf := new(bytes.Buffer)
	wzip := zip.NewWriter(buf)
	err = wzip.AddFS(os.DirFS(absPath))

	if err != nil {
		fmt.Fprintf(w, "Error writing zip archive: %s", err.Error())
		return
	}

	err = wzip.Close()

	if err != nil {
		fmt.Fprintf(w, "Error creating zip archive: %s", err.Error())
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
			DownloadUrl: "/download?path=" + url.QueryEscape(path.Join(root, nameInUrl)),
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
				DownloadUrl: "/download?path=" + url.QueryEscape(filePath),
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

type UploadHandler struct{}
func (h UploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Println("UploadHandler IN")

	err := r.ParseMultipartForm(10 << 20) // 10 MB

	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing multipart request: %s", err.Error()), http.StatusBadRequest)
		return;
	}

	rawFile, header, err := r.FormFile("file");

	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading multipart file: %s", err.Error()), http.StatusBadRequest)
		return;
	}

	defer rawFile.Close();

	destinationPath := r.FormValue("path")

	absFilePath, err := getAbsoluteMediaPath(filepath.Join(destinationPath, header.Filename))

	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting file path: %s", err.Error()), http.StatusBadRequest)
		return
	}

	_, err = os.Lstat(absFilePath)

	if err != nil && os.IsNotExist(err) {
		//
	} else if err != nil {
		http.Error(w, fmt.Sprintf("Error creating file: %s", err.Error()), http.StatusBadRequest)
		return
	} else {
		dir := filepath.Dir(absFilePath)
		base := filepath.Base(absFilePath)
		ext := filepath.Ext(base)

		noExtFilename := strings.TrimSuffix(base, ext)
		suffix := fmt.Sprintf("_%d", time.Now().UnixMilli());

		absFilePath = filepath.Join(dir, noExtFilename + suffix + ext)
	}

	fsFile, err := os.Create(absFilePath)

	if err != nil {
		http.Error(w, fmt.Sprintf("Error creating file: %s", err.Error()), http.StatusBadRequest)
		return
	}

	defer fsFile.Close()

	if _, err := io.Copy(fsFile, rawFile); err != nil {
		http.Error(w, "Error saving file", http.StatusInternalServerError)
		return
	}

	redirectUrl := r.FormValue("current_url")
	http.Redirect(w, r, redirectUrl, http.StatusSeeOther)
}

type MediaHandler struct{}
func (h MediaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Media handler hit with path: %s\n", r.URL.Path)

	handleError := func(err error) {
		fmt.Printf("Error in MediaHandler: %s\n", err.Error())
		errIsNotExist := os.IsNotExist(err)

		if errIsNotExist {
			// TODO: ir a pagina 404
			fmt.Fprintln(w, "Nothing here")
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
			fmt.Fprintf(w, "Error decoding the file name \"%s\": %s\n", fileName, err.Error())
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
		FsRelPath:   relPath,
		CurrentUrl:  currentUrl,
		ParentUrl:   parentDirPath,
		Files:       files,
		Breadcrumbs: breadcrumbs,
	}

	if err := tmpl.ExecuteTemplate(w, "base.tmpl", data); err != nil {
		log.Fatalln(err)
	}
}

const PORT = ":8080"

const mediaRootEnvVarKey = "YMS_MEDIA_ROOT"
var MEDIA_ROOT string = os.Getenv(mediaRootEnvVarKey)

func init() {
	if MEDIA_ROOT == "" {
		log.Fatalf("Env var %s not found", mediaRootEnvVarKey)
	}

	fmt.Printf("Root directory set to \"%s\"\n", MEDIA_ROOT)

	config := config.GetConfig()

	if config.Base64NamePrefix == "" {
		log.Fatalln("Invalid configuration: \"base_64_name_prefix\" is required")
	}

	fmt.Println("Config found:")
	fmt.Printf("- Base64NamePrefix: %s\n", config.Base64NamePrefix)
	fmt.Printf("- IgnorePaths: %v\n", config.IgnorePaths)
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "a")
	})

	mux.Handle("/", MainHandler{})
	mux.Handle("/media/", MediaHandler{})
	mux.Handle("/download/", DownloadHandler{})
	mux.Handle("/upload", UploadHandler{})
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	fmt.Printf("Listening on port %s\n", PORT)

	log.Panic(http.ListenAndServe(PORT, mux))
}
