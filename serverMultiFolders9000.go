// Created by Milind Patil 
// Syncaissa Systems Inc.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"path/filepath"
	"strings"
	"time"
    "io/ioutil"
    "strconv"
)

const TOTAL_MEDIA_FOLDERS = 10
const MEDIA_REFRESH_INTERVAL = 60 // in Seconds - 60*60 is 1 Hour - using constant

//var media_refresh_interval time.Duration = 60 // this is the Duration of the time, if using a variable

// define these as global
var listfiles []string

var files [TOTAL_MEDIA_FOLDERS][]string

//var juststarted bool
var mediafolderupdatetime [TOTAL_MEDIA_FOLDERS]time.Time // this is the actual Time

var totalfiles [TOTAL_MEDIA_FOLDERS]int
var basefolder = "/home/radio-systems/public/"

var mediafolder [TOTAL_MEDIA_FOLDERS]string
var mediafolderselected int

type FileObj struct {
	Filename string /* IMPORTANT: the fields should be uppercase, or they will be local and will not be sent over in POST json object */
}

// Issues resolved: Do not delete
// 1) The Struct needs to have all fields in first letter CAPS, so it is public  - or we get empty object {}.
// 2) In case the client needs lower case field - use the  `json:"songseq"` addition 
// 3) None of the fields in the struct can be of type int (use string)
// EXAMPLE:  Do not delete
// type receivedStruct struct {
//     Songseq string `json:"songseq"`     // none can be type of int (use string)
// }

type receivedStruct struct {
    Songseq string `json:"songseq"` 
}

func main() {

	//juststarted = true

	mediafolder[0] = basefolder + "media/*"
	mediafolder[1] = basefolder + "media1/*"
	mediafolder[2] = basefolder + "media2/*"
	mediafolder[3] = basefolder + "media3/*"
	mediafolder[4] = basefolder + "media4/*"
	mediafolder[5] = basefolder + "media5/*"
	mediafolder[6] = basefolder + "media6/*"
	mediafolder[7] = basefolder + "media7/*"
	mediafolder[8] = basefolder + "media8/*"
	mediafolder[9] = basefolder + "media9/*"
	

	files := make([][]string, TOTAL_MEDIA_FOLDERS) //the length is array is 5. Meaning say 5 media folders.

	for i := 0; i < len(mediafolder); i++ {
		files[i] = make([]string, 0) //the original length of the next dimensions of the array is 0, meaning no media files each of the media folders
	}

	// Until this point the above program/code will run only once during api api start-up
	// each subsequent GET/POST will hit only the following part of the program
	http.HandleFunc("/", handler)
	fmt.Println("go web server starting at port 9000")
	log.Fatal(http.ListenAndServe(":9000", nil))
}

func handler(w http.ResponseWriter, r *http.Request) {
	
	// First grab the values sent by client Code
    body, err := ioutil.ReadAll(r.Body)
    if err != nil {
        panic(err)
    }
    log.Println(string(body))
    var t receivedStruct
    err = json.Unmarshal(body, &t)
    if err != nil {
        //panic(err)
        fmt.Println("Issue with the JSON object: ", err) 
    }
    log.Println(t)   
    log.Println(t.Songseq) 
    songseq, err := strconv.Atoi(t.Songseq)   // convert string to int
    if err != nil {
        //panic(err)
        songseq = 0  //defaulting to 0
        fmt.Println("string to number convert issue or JSON not sent by client or POST not used: ", err) 
    }    

    fmt.Println("songseq: ", songseq) 
        
	// 1) randomly select a media folder
	mediafolderselected = randInt(0, len(mediafolder))

	// 2) OR let the client send a variable - say the sequentia song number that played (since stateless server, the client will have to save and increment that seq)
	// and select the folder using some function - say modulus
	//songseq = 1 // For testing the value has to be send by client in POST body
	//mediafolderselected = songseq % TOTAL_MEDIA_FOLDERS      // works so client song seq - controls the next song media folder - you may have advertisement after every 4th song, etc

	// 3) OR hard code one media folder
	//mediafolderselected = 0

	log.Println("Method received: ", r.Method )

	if r.Method == "GET" { /* why is request not POST, so not messing with GET - see GET1 */
		fmt.Fprintf(w, "Sorry, unable to access the requested page.")
	} else if r.Method == "OPTIONS" {
		log.Println("preflight or what?")
		w.Header().Set("Content-Type", "application/json")
		w.Write(nil)
	} else if r.Method == "POST" {
		/* access the files from the storage only first time, time is not initialized - compae with zero time 0001-01-01 00:00:00 +0000 UTC*/
		fmt.Println("Diff : ", time.Now().Sub(mediafolderupdatetime[mediafolderselected]).Seconds());
		diff := time.Now().Sub(mediafolderupdatetime[mediafolderselected])
		if mediafolderupdatetime[mediafolderselected] == time.Date(0001, 01, 01, 0, 0, 0, 0, time.UTC) || (diff > MEDIA_REFRESH_INTERVAL*time.Second) {
			//juststarted = false
			mediafolderupdatetime[mediafolderselected] = time.Now() //Just a indicator that this media folder was read from the disk and we do not ready for every request
			fmt.Printf("New read or media refresh- accessing file system. ")
			err := errors.New("Just to declare")
			err = nil
			listfiles, err = filepath.Glob(mediafolder[mediafolderselected])
			if err != nil {
				fmt.Printf("In error")
				log.Fatal(err)
			}
			//files[mediafolderselected] = append(files[mediafolderselected], "OSGN7 ") //use this if we are adding say one file per iteration.
			files[mediafolderselected] = listfiles // we have the complete list of files in the particular media folder, so just assign it.
			totalfiles[mediafolderselected] = len(files[mediafolderselected])
		} else {
			fmt.Printf("File names found in memory - ")
		}

		//fmt.Println(files) // contains a list of all files in the given directory
		randomnumber := randInt(0, totalfiles[mediafolderselected]-1)
		currentfile := files[mediafolderselected][randomnumber]
		currentfile = strings.Replace(currentfile, basefolder, "", -1) /* do not send the complete path - just all after public */
		currentfile = strings.Replace(currentfile, "..", "", -1)       /* security */
		fileObj := FileObj{Filename: currentfile}
		fmt.Println(fileObj)
		var js []byte
		var err = errors.New("")
		js, err = json.Marshal(fileObj)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	} else {
		fmt.Fprintf(w, "Sorry, unknown verb.")		
	}
}

func randInt(min int, max int) int {
	rand.Seed(time.Now().UTC().UnixNano())
	return min + rand.Intn(max-min)
}
