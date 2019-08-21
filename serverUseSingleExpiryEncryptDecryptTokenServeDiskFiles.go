package main

//http://12.345.678:9000/addfilesfromfolder  // to add folder both GET and API
//http://12.345.678:9000/reorderfiles   // to reorder files in the DB both GET and API for random order for users each time
//http://12.345.678:9000/newtimeexpirytoken  // get new token
//No longer user ->> http://12.345.678/encryptfilenames // to encrypt filenames with expiration both GET and API, not used as the media token is checked - common for all requests
//http://12.345.678:9000/media/bFophZjEsp9A+t09bE9nXJcDmYld2cO9m4VmhvR9ybiEA3XtyiCyaBP3NWJlVExM5pn6HGWv8KeDyHNqDPbTGliw==MsepbWVkaWEvc29sZmVnZ2lvL2cubXAz  // to server mp3s

//To server one random file from database (get the latest token value from http://12.345.678:9000/newtimeexpirytoken and replace here)
//http://12.345.678:9000/rdb/bFophZjEsp9A+t09bE9nXJcDmYld2cO9m4VmhvR9ybiEA3XtyiCyaBP3NWJlVExM5pn6HGWv8KeDyHNqDPbTGliw==
//To server one random file from fileserver - assume no database is up
//http://12.345.678:9000/rserver/bFophZjEsp9A+t09bE9nXJcDmYld2cO9m4VmhvR9ybiEA3XtyiCyaBP3NWJlVExM5pn6HGWv8KeDyHNqDPbTGliw==

//To run file and start web server: /home/cloud9/sio/goServer$ go run ./src/serverUseSingleExpiryEncryptDecryptTokenServePlanFiles.go
//Angular:
//To request media from say angular to serve media/mp3 files - first get the token - then append the efilename to it using the following pattern
//http://12.345.678:9000/media/tokenvalue + "Msep" + efilename
//Example: //http://12.345.678:9000/media/bFophZjEsp9A+t09bE9nXJcDmYld2cO9m4VmhvR9ybiEA3XtyiCyaBP3NWJlVExM5pn6HGWv8KeDyHNqDPbTGliw==MsepbWVkaWEvc29sZmVnZ2lvL2cubXAz

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	firebase "firebase.google.com/go"
	firestore "cloud.google.com/go/firestore"
	iterator "google.golang.org/api/iterator"
	b64 "encoding/base64"
	"fmt"
	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"
	"google.golang.org/api/option"
	"io"
	//"io/ioutil"
	"log"
	mathrand "math/rand" // since using crypto/rand, and still need simple math/rand so aliasing
	"net/http"
	"net/http/httputil"
	//"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//SETUP HERE VARIOUS CONFIG VALUES
const TOTAL_MEDIA_FOLDERS = 2     // make sure you make the list of folders below to match this.
const MEDIA_REFRESH_INTERVAL = 60*60 // in Seconds - for 1 minute use 60, for 1 hour use 60*60, recommended is 1 day, so it is not done many times, and someone may use direct link 1 day only, until it is expired - should be ok.
//const EXPIRATION_TIME = time.Now().UTC().Add(time.Hour*0 + time.Minute*1 + time.Second*0).Unix()    // did work so see the function for the value
const PORT = ":9000"
const BASEFOLDER = "/home/cloud9/sio/goServer/public/"

const FOLDER_SELECT_DIRECTIVE = 1 //See 1, 2 or 3 below

// define these as global
var listfilesinfolder []string

var files [TOTAL_MEDIA_FOLDERS][]string

//var juststarted bool
var mediafolderupdatetime [TOTAL_MEDIA_FOLDERS]time.Time // this is the actual Time

var totalfiles [TOTAL_MEDIA_FOLDERS]int

var mediafolder [TOTAL_MEDIA_FOLDERS]string
var mediafolderselected int

type FileObj struct {
	Filename string `json:"filename"` /* IMPORTANT: the fields should be uppercase, or they will be private (not public in go) and will not be sent over in POST json object - other packages cannot read it */
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

// type AdditionalInfo struct {
//     email string
//     someid  int
// }

var allfilesnamesBufferReadfromDB []string
var allfilesnamesBufferReadfromFileServer []string
   
func main() {

	router := httprouter.New()
	// decrypt and stream mp3
	// router.GET("/media/:encryptedfilename", checkPassPhraseThenStreamMediaHandler) //The audio SRC takes the URL and uses GET? - Leaving both here
	// router.GET("/media", needParametersError)                                      // if no file name is provide, show error/information, so the folders and not displayed
	// router.POST("/media/:encryptedfilename", checkPassPhraseThenStreamMediaHandler)
	// router.POST("/media", needParametersError) //so the folders and not displayed, if no file name is provide, show error/information

	// encrypt and send encrpted name, maybe used by hourly process? This to add the encrypted name to say a encrypted filename column to the firestore database
	// this helps us hide the real MP3 filename and also keep the validity of the url/filename for short period (1 day, etc), 
	// so users do not bypass the app and use and bookmark the MP3 direct url
	// router.GET("/encryptfilenames/:encryptedfilename", noParameterExpectedError)
	// router.GET("/encryptfilenames", encryptDBFileNamesWithExpirationLogic)
	// router.POST("/encryptfilenames/:encryptedfilename", noParameterExpectedError)
	// router.POST("/encryptfilenames", encryptDBFileNamesWithExpirationLogic)

	router.GET("/newtimeexpirytoken/:valuereceived", noParameterExpectedError)
	router.GET("/newtimeexpirytoken", generateNewTokenWithExpirationLogic)
	router.POST("/newtimeexpirytoken/:valuereceived", noParameterExpectedError)
	router.POST("/newtimeexpirytoken", generateNewTokenWithExpirationLogic)

	router.GET("/media/:tokenvalueandfullfilename", checkSingleTokenThenStreamMediaHandler) //The audio SRC takes the URL and uses GET? - Leaving both here
	router.GET("/media", needParametersError)                                      // if no file name is provide, show error/information, so the folders and not displayed
	router.POST("/media/:tokenvalueandfullfilename", checkSingleTokenThenStreamMediaHandler)
	router.POST("/media", needParametersError) //so the folders and not displayed, if no file name is provide, show error/information

	//If the go program also need to server the image or thumbnail of the song icon.
	router.GET("/image/:imagefilename", serveImageHandler)
	router.GET("/image", needParametersError)
	router.POST("/image/:imagefilename", serveImageHandler)
	router.POST("/image", needParametersError)

	//Initial load - adds all the files from a folder to database collection
	router.GET("/addfilesfromfolder/:name", noParameterExpectedError)
	router.GET("/addfilesfromfolder", readFolderAddDBRecords)
	router.POST("/addfilesfromfolder/:name", noParameterExpectedError)
	router.POST("/addfilesfromfolder", readFolderAddDBRecords)
	
	//Reorder records/documents randomly- so users get different flavor each time
	router.GET("/reorderfiles/:input", noParameterExpectedError)
	router.GET("/reorderfiles", reorderDocumentsRandomly)
	router.POST("/reorderfiles/:input", noParameterExpectedError)
	router.POST("/reorderfiles", reorderDocumentsRandomly)

	//Serve one random media file, using database
	router.GET("/rdb/:tokenvalue", randomdatabasefile)
	router.GET("/rdb", needParametersError)
	router.POST("/rdb/:tokenvalue", randomdatabasefile)
	router.POST("/rdb", needParametersError)

	//Serve one random media file, using no database, just file server
	router.GET("/rserver/:tokenvalue", randomserverfile)
	router.GET("/rserver", needParametersError)
	router.POST("/rserver/:tokenvalue", randomserverfile)
	router.POST("/rserver", needParametersError)

	//router.NotFound = http.FileServer(http.Dir(BASEFOLDER))  //use the following instead - more control
	//https://github.com/julienschmidt/httprouter/issues/93
	//Very Secure? - These are the only URI requests it responds to (in addition of above api and media), for rest 404 message
	router.NotFound = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println("r.URL.Path: ", r.URL.Path, " ip address: ", r.Header.Get("X-FORWARDED-FOR"))  // shows all uri's, log and see if any hacking attempts
		http.ServeFile(w, r, BASEFOLDER+"error.html")
		// leave the following for future reference, comment it so no other file is server by the golang server.
		// see all code in the commented portion below, keep it for code reference.
		// if r.URL.Path == "/" || r.URL.Path == "" || r.URL.Path == "/index.html" {
		// 	http.ServeFile(w, r, BASEFOLDER+"index.html")
		// } else if r.URL.Path == "/script.js" {
		// 	http.ServeFile(w, r, BASEFOLDER+"script.js")
		// } else if r.URL.Path == "/sioImageByGerdAltmannFromPixabay.jpg" {
		// 	http.ServeFile(w, r, BASEFOLDER+"sioImageByGerdAltmannFromPixabay.jpg")
		// } else if r.URL.Path == "/styles.css" {
		// 	http.ServeFile(w, r, BASEFOLDER+"styles.css")
		// } else if r.URL.Path == "/favicon.ico" {
		// 	http.ServeFile(w, r, BASEFOLDER+"favicon.ico")
		// } else if r.URL.Path == "/error.html" {
		// 	http.ServeFile(w, r, BASEFOLDER+"error.html")
		// } else {
		// 	w.Header().Set("Content-Type", "image/jpeg") // <-- set the content-type header
		// 	http.ServeFile(w, r, BASEFOLDER+"404Multi.jpg")
		// }
	})

	fmt.Println("Golang web server for serving media starting at port: ", PORT)

	router.OPTIONS("/", preFlight)
	// Until this point the above program/code will run only once during api api start-up / route initialization for different URIs, like /media or /encrypt, etc
	// each subsequent GET/POST will hit only the following part of the program
	log.Fatal(http.ListenAndServe(PORT, router)) // Start server! Specify router if using it
}

func preFlight(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	fmt.Fprint(w, "Welcome!\n")
}

func methodNotAllowedError(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	//fmt.Fprintf(w, "Unable to complete your request")
	w.Header().Set("Content-Type", "image/jpeg") // <-- set the content-type header
	http.ServeFile(w, r, BASEFOLDER+"404Multi.jpg")
}

func needParametersError(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	//fmt.Fprintf(w, "Parameters not provided")
	w.Header().Set("Content-Type", "image/jpeg") // <-- set the content-type header
	http.ServeFile(w, r, BASEFOLDER+"404Multi.jpg")
}

func noParameterExpectedError(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	fmt.Fprintf(w, "Parameters not expected for this function")
	w.Header().Set("Content-Type", "image/jpeg") // <-- set the content-type header
	http.ServeFile(w, r, BASEFOLDER+"404Multi.jpg")
}

// Function - Leave the funtion here - no longer needed, as using only a expiration token to see if it is valid for accessing files.
// func checkPassPhraseThenStreamMediaHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
// 	(w).Header().Set("Access-Control-Allow-Origin", "*")
//     (w).Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
//     (w).Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
    
// 	encryptedfilename := ps.ByName("encryptedfilename")
// 	fmt.Println("In checkPassPhraseThenStreamMediaHandler...")
// 	log.Println(encryptedfilename)

// 	myReplacer := strings.NewReplacer("tyiCyaB", "/") // the '/' was replaced earlier so it will not mess the URL, now restoring it before decrypt
// 	encryptedfilename = myReplacer.Replace(encryptedfilename)
// 	currentfilestring, err2 := b64.StdEncoding.DecodeString(encryptedfilename)

// 	if err2 != nil { // if the above is not base64, or any other error, file not found etc
// 		w.Header().Set("Content-Type", "image/jpeg") // <-- set the content-type header
// 		http.ServeFile(w, r, BASEFOLDER+"404Multi.jpg")
// 		fmt.Println("error:", err2)
// 		return
// 	}

// 	//Change the hardcoded paraphase value by say today's date, so same url may not be used for mp3 permanently
// 	//#TODO Also, remove the GET and use only POST to deliver song, so urls to songs are not bookmarked, bypassing the apps.
// 	filename := decrypt(currentfilestring, "password")
// 	filenameandPath := BASEFOLDER + string(filename)
//     // fmt.Println("Decrypted filename and Path = ", filenameandPath)

// 	if string(filename) == "MY_URL_FILENAME_EXPIRED" || string(filename) == "MY_URL_INCORRECT_LENGTH" {
// 		// w.Header().Set("Content-Type", "text/plain; charset=utf-8")
// 		// w.WriteHeader(http.StatusNotFound) // StatusNotFound = 404
// 		// w.Write([]byte("Resource Invalid/expired."))
// 		w.Header().Set("Content-Type", "image/jpeg") // <-- set the content-type header
// 		//http.ServeFile(w, r, BASEFOLDER+"404Multi.jpg")
// 		http.ServeFile(w, r, BASEFOLDER+"expired.gif")
// 	} else {
// 		w.Header().Set("Content-Type", "audio/mpeg") // the golang header for streaming audio file
// 		http.ServeFile(w, r, filenameandPath)        // the golang is now actually streaming audio file.
// 		logMediaPlayed(w,r,ps,string(filename))
// 		log.Println("playing:",filenameandPath)
// 	}
// }

func checkSingleTokenThenStreamMediaHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	(w).Header().Set("Access-Control-Allow-Origin", "*")
    (w).Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
    (w).Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
    
    fmt.Println("In checkSingleTokenThenStreamMediaHandler...")

	tokenvalueandfullfilename := ps.ByName("tokenvalueandfullfilename");
	parts := strings.Split(tokenvalueandfullfilename, "Msep")
	//tokenvalue := parts[len(parts)-1]  // If need the last part 
	tokenvalue := parts[0]             // If need the first part
	filename := parts[1]

	myReplacerFile := strings.NewReplacer("Fsep", "/") // the '/' was replaced earlier so it will not mess the URL, now restoring it before decrypt
	filename = myReplacerFile.Replace(filename)

	log.Println("tokenvalue:", tokenvalue)
	log.Println("filename:", filename)
	
	// If the token is say: 3g6rDYRAKptyiCyaBh2QxkC7uV3HIw97TrlLBnyZAG7ZKViJ0FIQ+8TuN9lx62A80Y3mDCzQxwnCiBMX1aNoZIGA==
	// and full filename is: media/sio/g.mp3
	// Before sending the request concatenate the token and full filename with "Msep" and replace the '/' with "Fsep" in the filename
	// then the full string will be: 3g6rDYRAKptyiCyaBh2QxkC7uV3HIw97TrlLBnyZAG7ZKViJ0FIQ+8TuN9lx62A80Y3mDCzQxwnCiBMX1aNoZIGA==MsepmediaFsepsioFsepg.mp3
	// http://12.345.678:9000/media/3g6rDYRAKptyiCyaBh2QxkC7uV3HIw97TrlLBnyZAG7ZKViJ0FIQ+8TuN9lx62A80Y3mDCzQxwnCiBMX1aNoZIGA==MsepmediaFsepsioFsepg.mp3  

	 	 // Leave this here - only if using query string ?x=y&z=a etc
	 	 // tokenvalueget, ok := r.URL.Query()["tokenvalue"]
	     //   if !ok || len(tokenvalueget[0]) < 1 {
	     //       log.Println("Url Param 'key' is missing")
		 //       return
		 //   }
		 //   // Query()["key"] will return an array of items, 
		 //   // we only want the single item.
		 //   token := tokenvalueget[0]
		 //   log.Println("Url Param 'token' is: " + string(token))
		 // filenameget, ok := r.URL.Query()["filename"]
		 //   if !ok || len(filenameget[0]) < 1 {
		 //       log.Println("Url Param 'filename' is missing")
		 //       return
		 //   }
		 //   // Query()["key"] will return an array of items, 
		 //   // we only want the single item.
		 //   file := filenameget[0]
		 //   log.Println("Url Param 'file' is: " + string(file))	    

	myReplacer := strings.NewReplacer("tyiCyaB", "/") // the '/' was replaced earlier for base64 token so it will not mess the URL, now restoring it before decrypt
	tokenvalue = myReplacer.Replace(tokenvalue)
	currenttokenvalue, err2 := b64.StdEncoding.DecodeString(tokenvalue)
	
	myReplacer2 := strings.NewReplacer("tyiCyaB", "/") // the '/' was replaced earlier for base64 full filename so it will not mess the URL, now restoring it before decrypt
	filename = myReplacer2.Replace(filename)
	filenamevalue, err3 := b64.StdEncoding.DecodeString(filename)	

	if err2 != nil { // if the above is not base64, or any other error, file not found etc
		w.Header().Set("Content-Type", "image/jpeg") // <-- set the content-type header
		http.ServeFile(w, r, BASEFOLDER+"404Multi.jpg")
		fmt.Println("error with token:", err2)
		return
	}

	if err3 != nil { // if the above is not base64, or any other error, file not found etc
		w.Header().Set("Content-Type", "image/jpeg") // <-- set the content-type header
		http.ServeFile(w, r, BASEFOLDER+"404Multi.jpg")
		fmt.Println("error with full filename:", err3)
		return
	}

	//Change the hardcoded paraphase value by say today's date, so same url may not be used for mp3 permanently
	//#TODO Also, remove the GET and use only POST to deliver song, so urls to songs are not bookmarked, bypassing the apps.
	token_result := decryptIsTokenExpired(currenttokenvalue, "password")

	if string(token_result) == "TOKEN_EXPIRED" || string(token_result) == "TOKEN_INCORRECT_LENGTH" {
		// w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		// w.WriteHeader(http.StatusNotFound) // StatusNotFound = 404
		// w.Write([]byte("Resource Invalid/expired."))
		w.Header().Set("Content-Type", "image/jpeg") // <-- set the content-type header
		//http.ServeFile(w, r, BASEFOLDER+"404Multi.jpg")
		http.ServeFile(w, r, BASEFOLDER+"expired.gif")
	} else {  // here "TOKEN_VALID"
	    filenameandPath := BASEFOLDER + string(filenamevalue)
		w.Header().Set("Content-Type", "audio/mpeg") // the golang header for streaming audio file
		http.ServeFile(w, r, filenameandPath)        // the golang is now actually streaming audio file.
		logMediaPlayed(w,r,ps,string(filenamevalue))
		log.Println("Started Playing:",filenameandPath)
	}
}

func randomdatabasefile(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	(w).Header().Set("Access-Control-Allow-Origin", "*")
    (w).Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
    (w).Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
    
    fmt.Println("In randomdatabasefile...")

	tokenvalue := ps.ByName("tokenvalue");
	filename := "bWVkaWEvc29sZmVnZ2lvL2cubXAz" //#TODO get random filename from database

    if len(allfilesnamesBufferReadfromDB) < 5 {	
		log.Println("Reading from DB as len(allfilesnamesBufferReadfromDB):", len(allfilesnamesBufferReadfromDB))
		sa := option.WithCredentialsFile(BASEFOLDER + "../config/solfegg-io-firebase-adminsdk-oqa9s-d34c6f7b32.json")
		app, err := firebase.NewApp(context.Background(), nil, sa)
		if err != nil {
			log.Fatalln(err)
		}
		client, err := app.Firestore(context.Background())
		if err != nil {
			log.Fatalln(err)
		}
	   defer client.Close()
	   //var allfilesnamesBufferReadfromDB []string  // moved to global, so next request will have values.
	   allfilesnamesBufferReadfromDB = make([]string, 0, 0)
	   iter := client.Collection("sio_prod_master").Documents(context.Background())
	   i := 0
	   for {
	        doc, err := iter.Next()
	        if err == iterator.Done {
	                break
	        }
	        if err != nil {
	                //return err
	                fmt.Println(err)
	        }
						// fmt.Println("i = ")
						// fmt.Println(i)
						allfilesnamesBufferReadfromDB = append(allfilesnamesBufferReadfromDB, fmt.Sprint(doc.Data()["efilename"]))  // adding elements to slice dynamically
					 //   fmt.Println("allfilesnamesBufferReadfromDB = ")
						// fmt.Println(allfilesnamesBufferReadfromDB[i])
					    // orderseq := doc.Data()["orderseq"] // in case you need to use it
						// if orderseq == randomnumber {
						// 	filename = fmt.Sprint(doc.Data()["efilename"])   // error with doc.Data()["efilename"] was need type assertion, so using fmt.Sprint(doc.Data()["efilename"])
						// }
			
	   		i++;
		}
		} else {
				log.Println("Filenames exists in the slice as len(allfilesnamesBufferReadfromDB):", len(allfilesnamesBufferReadfromDB))
		}
   
    randomnumber := randInt(0, len(allfilesnamesBufferReadfromDB)-1) 
    //fmt.Println("randomnumber = ")
    //fmt.Println(randomnumber)
    filename = allfilesnamesBufferReadfromDB[randomnumber]
    //fmt.Println("filename = " + filename)
	myReplacerFile := strings.NewReplacer("Fsep", "/") // the '/' was replaced earlier so it will not mess the URL, now restoring it before decrypt
	filename = myReplacerFile.Replace(filename)

	// log.Println("tokenvalue:", tokenvalue)
	log.Println("filename:", filename)
	
	myReplacer := strings.NewReplacer("tyiCyaB", "/") // the '/' was replaced earlier for base64 token so it will not mess the URL, now restoring it before decrypt
	tokenvalue = myReplacer.Replace(tokenvalue)
	currenttokenvalue, err2 := b64.StdEncoding.DecodeString(tokenvalue)
	
	myReplacer2 := strings.NewReplacer("tyiCyaB", "/") // the '/' was replaced earlier for base64 full filename so it will not mess the URL, now restoring it before decrypt
	filename = myReplacer2.Replace(filename)
	filenamevalue, err3 := b64.StdEncoding.DecodeString(filename)	

	if err2 != nil { // if the above is not base64, or any other error, file not found etc
		w.Header().Set("Content-Type", "image/jpeg") // <-- set the content-type header
		http.ServeFile(w, r, BASEFOLDER+"404Multi.jpg")
		fmt.Println("error with token:", err2)
		return
	}

	if err3 != nil { // if the above is not base64, or any other error, file not found etc
		w.Header().Set("Content-Type", "image/jpeg") // <-- set the content-type header
		http.ServeFile(w, r, BASEFOLDER+"404Multi.jpg")
		fmt.Println("error with full filename:", err3)
		return
	}

	token_result := decryptIsTokenExpired(currenttokenvalue, "password")

	if string(token_result) == "TOKEN_EXPIRED" || string(token_result) == "TOKEN_INCORRECT_LENGTH" {
		w.Header().Set("Content-Type", "image/jpeg") // <-- set the content-type header
		http.ServeFile(w, r, BASEFOLDER+"expired.gif")
	} else {  // here "TOKEN_VALID"
	    filenameandPath := BASEFOLDER + string(filenamevalue)
		w.Header().Set("Content-Type", "audio/mpeg") // the golang header for streaming audio file
		http.ServeFile(w, r, filenameandPath)        // the golang is now actually streaming audio file.
		logMediaPlayed(w,r,ps,string(filenamevalue))
		log.Println("Started Playing:",filenameandPath)
	}
}

func randomserverfile(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	(w).Header().Set("Access-Control-Allow-Origin", "*")
    (w).Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
    (w).Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
    
    fmt.Println("In randomserverfile...")

	tokenvalue := ps.ByName("tokenvalue");
	filename := "bWVkaWEvc29sZmVnZ2lvL2cubXAz" //#TODO get random filename from File SERVER

	if len(allfilesnamesBufferReadfromFileServer) < 5 {	
	log.Println("Reading from File Server as len(allfilesnamesBufferReadfromFileServer):", len(allfilesnamesBufferReadfromFileServer))
			// Read file server
  			// upload one folder at a time
			FOLDER2BADDED := "media/sio/*"   // the * is important - it means read ALL file names from the folder and store list in the variable.
			//	FOLDER2BADDED := "media/meditation/*"  // the * is important - it means read ALL file names from the folder and store list in the variable. 
			fullname := BASEFOLDER+FOLDER2BADDED 

			filelist, err := filepath.Glob(fullname)
			allfilesnamesBufferReadfromFileServer = filelist
			if err != nil {
				fmt.Printf("In error")
				log.Fatal(err)
			}
	} else {
		log.Println("Filenames exists in the slice as len(allfilesnamesBufferReadfromFileServer):", len(allfilesnamesBufferReadfromFileServer))
	}
	
    randomnumber := randInt(0, len(allfilesnamesBufferReadfromFileServer)-1) 
    filename = allfilesnamesBufferReadfromFileServer[randomnumber]

	myReplacerFile := strings.NewReplacer("Fsep", "/") // the '/' was replaced earlier so it will not mess the URL, now restoring it before decrypt
	filename = myReplacerFile.Replace(filename)

	// log.Println("tokenvalue:", tokenvalue)
	log.Println("filename:", filename)
	
	myReplacer := strings.NewReplacer("tyiCyaB", "/") // the '/' was replaced earlier for base64 token so it will not mess the URL, now restoring it before decrypt
	tokenvalue = myReplacer.Replace(tokenvalue)
	currenttokenvalue, err2 := b64.StdEncoding.DecodeString(tokenvalue)
	
	myReplacer2 := strings.NewReplacer("tyiCyaB", "/") // the '/' was replaced earlier for base64 full filename so it will not mess the URL, now restoring it before decrypt
	filename = myReplacer2.Replace(filename)
	filenamevalue := filename // when server from file server, it is not encoded          //filenamevalue, err3 :=  b64.StdEncoding.DecodeString(filename)	

	if err2 != nil { // if the above is not base64, or any other error, file not found etc
		w.Header().Set("Content-Type", "image/jpeg") // <-- set the content-type header
		http.ServeFile(w, r, BASEFOLDER+"404Multi.jpg")
		fmt.Println("error with token:", err2)
		return
	}

	// if err3 != nil { // if the above is not base64, or any other error, file not found etc
	// 	w.Header().Set("Content-Type", "image/jpeg") // <-- set the content-type header
	// 	http.ServeFile(w, r, BASEFOLDER+"404Multi.jpg")
	// 	fmt.Println("error with full filename:", err3)
	// 	return
	// }

	token_result := decryptIsTokenExpired(currenttokenvalue, "password")

	if string(token_result) == "TOKEN_EXPIRED" || string(token_result) == "TOKEN_INCORRECT_LENGTH" {
		w.Header().Set("Content-Type", "image/jpeg") // <-- set the content-type header
		http.ServeFile(w, r, BASEFOLDER+"expired.gif")
	} else {  // here "TOKEN_VALID"
	    //filenameandPath := BASEFOLDER + string(filenamevalue)
	    filenameandPath := string(filenamevalue)
		w.Header().Set("Content-Type", "audio/mpeg") // the golang header for streaming audio file
		http.ServeFile(w, r, filenameandPath)        // the golang is now actually streaming audio file.
		logMediaPlayed(w,r,ps,string(filenamevalue))
		log.Println("Started Playing:",filenameandPath)
	}
}

func logMediaPlayed(w http.ResponseWriter, r *http.Request, ps httprouter.Params, filename string) {

	sa := option.WithCredentialsFile(BASEFOLDER + "../config/solfegg-io-firebase-adminsdk-oqa9s-d34c6f7b32.json")

	app, err := firebase.NewApp(context.Background(), nil, sa)
	if err != nil {
		log.Fatalln(err)
	}

	client, err := app.Firestore(context.Background())
	if err != nil {
		log.Fatalln(err)
	}

	defer client.Close()
	
	_, _, err = client.Collection("sio_prod_activity").Add(context.Background(), map[string]interface{}{
		"file":    filename,
		"ip":      r.Header.Get("X-FORWARDED-FOR"), //r.RemoteAddr, WORKING in DB Insert
		"created": time.Now(),
	})
	if err != nil {
		log.Fatalf("Failed adding: %v", err)
	}
}		

func dumpHTTPRequest(r *http.Request) { // To debug your HTTP requests, import the net/http/httputil package and pass *http.Request
	output, err := httputil.DumpRequest(r, true)
	if err != nil {
		fmt.Println("Error dumping request:", err)
		return
	}
	fmt.Println("*************start dump/show request")
	fmt.Println(string(output))
	fmt.Println("*************end dump/show request")
}

func randInt(min int, max int) int {
	//fmt.Printf("In Random Integer randInt min = %d max = %d\n", min, max)
	mathrand.Seed(time.Now().UTC().UnixNano())
	return min + mathrand.Intn(max-min)
}

//func init() { Reader = &devReader{name: "/dev/urandom"} }

func createHash(key string) string {
	hasher := md5.New()
	hasher.Write([]byte(key))
	return hex.EncodeToString(hasher.Sum(nil))
}

//https://www.thepolyglotdeveloper.com/2018/02/encrypt-decrypt-data-golang-application-crypto-packages/
func encrypt(inputValue []byte, passphrase string) []byte {
	block, _ := aes.NewCipher([]byte(createHash(passphrase)))
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(err.Error())
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		panic(err.Error())
	}

	//Begin- Comment the next few lines to remove the URL/Link Expiry feature - Code to check the validity of the URL or filename - was it created within 24 Hours, etc?
	// get UTC (greenwich) time and add 24 hours - so check this at decrypt and make it valid if it is less than 24 hours - valid for 24 hours
	//addCurrentDatePlusPredefinedValue := time.Now().UTC().Add(time.Hour*24 + time.Minute*0 + time.Second*0)
	// Logic - get UTC Time, add 24 Hours, change to base10 integer, change to string, change to byte array,
	// append the first 10 chars to the filename, then encrypt the whole string (byte array)
	// On the other side, reverse the logic and get the time, if the time is <= current time - play the song. else return message or 404
	//addCurrentDatePlusPredefinedValue := EXPIRATION_TIME  // did not work, so adding below
	addCurrentDatePlusPredefinedValue := time.Now().UTC().Add(time.Hour*24 + time.Minute*0 + time.Second*0).Unix()  // 24 hours
	//addCurrentDatePlusPredefinedValue := time.Now().UTC().Add(time.Hour*0 + time.Minute*1 + time.Second*0).Unix()    // 1 miniute
	inputValue = append([]byte(strconv.FormatInt(addCurrentDatePlusPredefinedValue, 10)), inputValue...)
	//later do not forget to remove the first 10 characters and check to see if the timestamp is < current time
	//End- check for URL/filename expiry feature

	ciphertext := gcm.Seal(nonce, nonce, inputValue, nil)
	return ciphertext
}

// Function - Leave the funtion here - no longer needed, as using only a expiration token to see if it is valid for accessing files.
// func decrypt(data []byte, passphrase string) []byte {
// 	if len(data) > 12 {
// 		key := []byte(createHash(passphrase))
// 		block, err := aes.NewCipher(key)
// 		if err != nil {
// 			panic(err.Error())
// 		}
// 		gcm, err := cipher.NewGCM(block)
// 		if err != nil {
// 			panic(err.Error())
// 		}
// 		nonceSize := gcm.NonceSize()
// 		nonce, ciphertext := data[:nonceSize], data[nonceSize:]
// 		plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
// 		if err != nil {
// 			panic(err.Error())
// 		}

// 		//Begin- Comment the next lines if you do not wish to have the functionality for valid 24 hour (or any other) validation.
// 		//The first 10 elements of the []byte is time sent for validation checking.
// 		//The remaining is the filename!, check to see the time received is less than current time, if less pass plaintext back, else error.
// 		//timepart := plaintext[0:10]
// 		currenttime := strconv.FormatInt(time.Now().UTC().Unix(), 10)
// 		timevaliduptopart := "0000000000"
// 		if len(plaintext) > 12 {
// 			timevaliduptopart = string(plaintext[0:10]) // get the first 10 bytes that have the timestamp this was created + XYZ (24) Hours
// 			filenamepart := plaintext[10:]
// 			plaintext = filenamepart
// 		}
// 		//End- check for URL/filename expiry feature
	
// 	   ///////////// TESTING ALWAYS RETURN media mp3
// 	   log.Println("playing - NO CHECKING expired condition");
// 		if timevaliduptopart > currenttime || 1==1 {
// 			return plaintext
// 		} else {
// 			return []byte("MY_URL_FILENAME_EXPIRED")
// 		}

// 	} else {
// 		return []byte("MY_URL_INCORRECT_LENGTH")
// 	}

// }

func decryptIsTokenExpired(data []byte, passphrase string) []byte {
	
	//	return []byte("TOKEN_VALID") //////////// remove this line completely
	
	if len(data) > 12 {
		key := []byte(createHash(passphrase))
		block, err := aes.NewCipher(key)
		if err != nil {
			panic(err.Error())
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			panic(err.Error())
		}
		nonceSize := gcm.NonceSize()
		nonce, ciphertext := data[:nonceSize], data[nonceSize:]
		plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
		if err != nil {
			panic(err.Error())
		}

		//Begin- Comment the next lines if you do not wish to have the functionality for valid 24 hour (or any other) validation.
		//The first 10 elements of the []byte is time sent for validation checking.
		//The remaining is the for future use, check to see the time received is less than current time, if less pass plaintext back, else error.
		//timepart := plaintext[0:10]
		currenttime := strconv.FormatInt(time.Now().UTC().Unix(), 10)
		timevaliduptopart := "0000000000"
		if len(plaintext) > 12 {
			timevaliduptopart = string(plaintext[0:10]) // get the first 10 bytes that have the timestamp this was created + XYZ (24) Hours
			// remaining was [10:] used for file name - but not here, remaining is the for future use
		}
		//End- check for URL/filename expiry feature
	
		if timevaliduptopart > currenttime {  // ">"" is correct
			return []byte("TOKEN_VALID")
		} else {
			return []byte("TOKEN_EXPIRED")
		}

	} else {
		return []byte("TOKEN_INCORRECT_LENGTH")
	}
}

func serveImageHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	imagefilename := ps.ByName("imagefilename")
    fmt.Println(imagefilename)  
	w.Header().Set("Content-Type", "image/jpeg") // <-- set the content-type header
	//http.ServeFile(w, r, BASEFOLDER+"radio.jpg")	
	http.ServeFile(w, r, BASEFOLDER+imagefilename)		
}

// Function - Leave the funtion here - no longer needed, as using only a expiration token to see if it is valid for accessing files.
// func encryptDBFileNamesWithExpirationLogic(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {

// 	//dumpHTTPRequest(r) // To debug your HTTP requests, import the net/http/httputil package and pass *http.Request
// 	fmt.Println("in encryptFileNameWithExpiration...")

// 	//encryptedfilename := ps.ByName("encryptedfilename") // no filename is sent - we are doing all files on firestore table/encrypted column
// 	// Use a service account
// 	sa := option.WithCredentialsFile(BASEFOLDER + "../config/solfegg-io-firebase-adminsdk-oqa9s-d34c6f7b32.json")

// 	app, err := firebase.NewApp(context.Background(), nil, sa)
// 	if err != nil {
// 		log.Fatalln(err)
// 	}

// 	client, err := app.Firestore(context.Background())
// 	if err != nil {
// 		log.Fatalln(err)
// 	}

// 	defer client.Close()

// 	// First grab the values sent by client Code
// 	body, err := ioutil.ReadAll(r.Body)
// 	if err != nil {
// 		panic(err)
// 	}
// 	//log.Println(string(body))
// 	var t receivedStruct
// 	err = json.Unmarshal(body, &t)
// 	if err != nil {
// 		//panic(err)
// 		fmt.Println("Issue with the JSON object: ", err)
// 	}
// 	//log.Println(t)
// 	///////////log.Println("t.Songseq = ", t.Songseq)
// 	// songseq, err := strconv.Atoi(t.Songseq) // convert string to int
// 	// if err != nil {
// 	// 	//panic(err)
// 	// 	songseq = 0 //defaulting to 0
// 	// 	fmt.Println("string to number convert issue or JSON not sent by client or POST not used: ", err)
// 	// }

// 	//log.Println("Method received: ", r.Method)

// 	if r.Method == "OPTIONS" {
// 		log.Println("preflight or what?")
// 		w.Header().Set("Content-Type", "application/json")
// 		w.Write(nil)
// 	} else if r.Method == "POST" || r.Method == "GET" {
//   fmt.Fprint(w, "Encrypting filenames\n")
//   fmt.Println("Encrypting filenames\n")
//   iter := client.Collection("sio_prod_master").Documents(context.Background())
//   for {
//         doc, err := iter.Next()
//         if err == iterator.Done {
//                 break
//         }
//         if err != nil {
//                 //return err
//                 fmt.Println(err)
//         }
//         filename := doc.Data()["filenamewithpath"]
//         fmt.Println(filename)
//         // go SDK could not get the id of the document in select - so as a workaround added the key "workaroundgoid" as a field to the collection with value = id of doc.
//         id := doc.Data()["workaroundgoid"]
//         // The error was" cannot convert filename (type interface {}) to type []byte: need type assertion, fixed by replacing filename by fmt.Sprint(filename)
// 		ciphercurrentfile := encrypt([]byte(fmt.Sprint(filename)), "password")
//         ciphercurrentfilestring := b64.StdEncoding.EncodeToString(ciphercurrentfile)
//         myReplacer := strings.NewReplacer("/", "tyiCyaB") // replace "/"" by predefined string say tyiCyaB
//         ciphercurrentfilestring = myReplacer.Replace(ciphercurrentfilestring)
//         efilename := ciphercurrentfilestring

//         fmt.Println(filename)        
//         fmt.Println(efilename)        
        
        
//         // check this out - may not need the workaroundgoid field <<<<<<<<<<<<<<<<<<<<<<<<<<<<
//         // https://godoc.org/cloud.google.com/go/firestore
//         //_, err = ca.Update(ctx, []firestore.Update{{Path: "capital", Value: "Sacramento"}})
        
// 					//updating the efilename after encrypting
// 					_, err = client.Collection("sio_prod_master").Doc(fmt.Sprint(id)).Set(context.Background(), map[string]interface{}{
// 					        "efilename": efilename,
// 					}, firestore.MergeAll)
					
// 					if err != nil {
// 					        // Handle any errors in an appropriate way, such as returning them.
// 					        log.Printf("An error has occurred: %s", err)
// 					}
//   }
//  }
// }

func generateNewTokenWithExpirationLogic(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {

	//dumpHTTPRequest(r) // To debug your HTTP requests, import the net/http/httputil package and pass *http.Request
     	fmt.Println("in generateNewTokenWithExpirationLogic...")
		type moreInfo struct {
		    Email string     //make sure the fields are uppercase, so info is public and is sent.
    		Someid  int
		}
		someadditionalinfo := moreInfo{"account@email.com", 100}  //if you wish to send some addition value in addition to the time stamp (added in the encrypt function)
		ciphercurrenttoken := encrypt([]byte(fmt.Sprint(someadditionalinfo)), "password")
        ciphercurrenttokenstring := b64.StdEncoding.EncodeToString(ciphercurrenttoken)
        myReplacer := strings.NewReplacer("/", "tyiCyaB") // replace "/"" by predefined string say tyiCyaB
        ciphercurrenttokenstring = myReplacer.Replace(ciphercurrenttokenstring)
		// Now send this ciphercurrenttokenstring to the client, so it may be sent back to the server with each media request. Play media only if this token is not expired.
		
    	w.Header().Set("Content-Type", "application/json")
    	w.WriteHeader(http.StatusCreated)
    	json.NewEncoder(w).Encode(ciphercurrenttokenstring)		
}

func reorderDocumentsRandomly(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
   fmt.Println("Reordering records")
   fmt.Fprint(w, "Reordering records\n")

	sa := option.WithCredentialsFile(BASEFOLDER + "../config/solfegg-io-firebase-adminsdk-oqa9s-d34c6f7b32.json")

	app, err := firebase.NewApp(context.Background(), nil, sa)
	if err != nil {
		log.Fatalln(err)
	}

	client, err := app.Firestore(context.Background())
	if err != nil {
		log.Fatalln(err)
	}

	defer client.Close()

   iter := client.Collection("sio_prod_master").Documents(context.Background())
   for {
        doc, err := iter.Next()
        if err == iterator.Done {
                break
        }
        if err != nil {
                //return err
                fmt.Println(err)
        }
                    id := doc.Data()["workaroundgoid"]
                    //orderseq := doc.Data()["orderseq"] // in case you need to use it
                    randomnumber := randInt(0, 99999999) 
					//updating the efilename after encrypting
					_, err = client.Collection("sio_prod_master").Doc(fmt.Sprint(id)).Set(context.Background(), map[string]interface{}{
					        "orderseq": randomnumber,
					}, firestore.MergeAll)
					
					if err != nil {
					        // Handle any errors in an appropriate way, such as returning them.
					        log.Printf("An error has occurred: %s", err)
					}
  }
}	

// use this for original uplaod of file names (names, path only not actual mp3) to firebase during startup, or later to upload more songs (names only) to firebase from new folders
func readFolderAddDBRecords(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	
			sa := option.WithCredentialsFile(BASEFOLDER + "../config/solfegg-io-firebase-adminsdk-oqa9s-d34c6f7b32.json")
			app, err1 := firebase.NewApp(context.Background(), nil, sa)
			if err1 != nil {
				log.Fatalln(err1)
			}
		
			client, err1 := app.Firestore(context.Background())
			if err1 != nil {
				log.Fatalln(err1)
			}
		
			defer client.Close()

			fmt.Printf("New read file names in folder and upload to db- accessing file system. ")
			err := errors.New("Just to declare, to be used below")
			err = nil
			// upload one folder at a time
			FOLDER2BADDED := "media/sio/*"   // the * is important - it means read ALL file names from the folder and store list in the variable.
		//	FOLDER2BADDED := "media/meditation/*"  // the * is important - it means read ALL file names from the folder and store list in the variable. 
			fullname := BASEFOLDER+FOLDER2BADDED   
			listfilesinfolder, err = filepath.Glob(fullname)
			fmt.Printf("Fullname========= ")
			fmt.Printf(fullname )
			if err != nil {
				fmt.Printf("In error")
				log.Fatal(err)
			}

			//logic to get the filename by using logic to get string part after the last slash
			myReplacer := strings.NewReplacer(BASEFOLDER, "") 
			for i := 0; i < len(listfilesinfolder); i++ {
		        filenamewithtoplevelfolders := myReplacer.Replace(listfilesinfolder[i])
		        fmt.Fprint(w, filenamewithtoplevelfolders)	
    	        fmt.Fprint(w, "\n")
    	        
				parts := strings.Split(filenamewithtoplevelfolders, "/")
				justfilename := parts[len(parts)-1] // last segment is the file name
		        fmt.Fprint(w, justfilename)					
				fmt.Fprint(w, "\n")
				
				//get the final folder, use it as the default for the music classification (Example: meditation, sio, etc - refine later if required)
				classification := parts[len(parts)-2]   // second to last segment is the final folder name
		        fmt.Fprint(w, classification)					
				fmt.Fprint(w, "\n")
				
				// check if the file is valid, else continue rest of the loop
				parts2 := strings.Split(justfilename, ".")				
				fileextension := parts2[len(parts2)-1]
				if fileextension == "mp3" || fileextension == "MP3" || fileextension == "wav" || fileextension == "WAV" {
			        fmt.Fprint(w, "Above good filename")					
					fmt.Fprint(w, "\n")
				} else {
			        fmt.Fprint(w, "Above bad filename, extension invalid or folder selected")					
					fmt.Fprint(w, "\n")
					continue
				}
				
				//encrypt filenames  **** IMPORTANT - use this logic to encrypt the filename and prepend the date etc using the encrypt() custom function, NOT using currently so commented off
				// ciphercurrentfile := encrypt([]byte(fmt.Sprint(filenamewithtoplevelfolders)), "password")
				// ciphercurrentfilestring := b64.StdEncoding.EncodeToString(ciphercurrentfile)
				// myReplacer := strings.NewReplacer("/", "tyiCyaB") // replace "/"" by predefined string say tyiCyaB
				// ciphercurrentfilestring = myReplacer.Replace(ciphercurrentfilestring)

				// Just using the base64 encryption. 
				base64currentfilestring := b64.StdEncoding.EncodeToString([]byte(filenamewithtoplevelfolders))   // see []byte(... added compare to above)
				myReplacer := strings.NewReplacer("/", "tyiCyaB") // replace "/"" by predefined string say tyiCyaB
				base64currentfilestring = myReplacer.Replace(base64currentfilestring)

				b := make([]byte, 16)
				_, err := rand.Read(b)
				if err != nil {
				    log.Fatal(err)
				}
				//uuid := fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
				uuid :=  fmt.Sprintf("%x%x%x%x%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])  // with no dashes
				randomnumber := randInt(0, 99999999) 
				
				// #TODO extra checks: see if the filename already exists, then do not add the same file again.
				

				// Important note: why are we duplicating the firestore id to a field: workaroundgoid
				// this is because when selecting all documents in loop, the id is not selected (and snapshot logic does not work currently in go)
				// need the id to update some fields in the same loop - say the encrypted file name is updated periodically
				 _, err = client.Collection("sio_prod_master").Doc(uuid).Set(context.Background(), map[string]interface{}{
					"workaroundgoid": uuid,
					"filename": justfilename,
					"filenamewithpath": filenamewithtoplevelfolders,
					"efilename": base64currentfilestring, /*ciphercurrentfilestring, use this if encryping with timestamp - not using now. Just using base64 encryption and a separate valid standalone expiry token*/
					"description": "Music just for you!",
					"title": "Sangeet.cc Music",
					"classification": classification,
					"orderseq": randomnumber,
					"status": "A",
					"created": time.Now(),
				},firestore.MergeAll)
				if err != nil {
					log.Fatalf("Failed adding: %v", err)
				}

				// Leave this here - commented, the issue was that could not use the custom ID to add new record, the above logic was successful 
				// _, _, err = client.Collection("sio_prod_master").Add(context.Background(), map[string]interface{}{
				// 	"workaroundgoid": uuid,
				// 	"filename": justfilename,
				// 	"filenamewithpath": filenamewithtoplevelfolders,
				// 	"efilename": ciphercurrentfilestring,
				// 	"description": "",
				// 	"title": "",
				// 	"created": time.Now(),
				// })
				// if err != nil {
				// 	log.Fatalf("Failed adding: %v", err)
				// }				
		
			}            

}

