package main

// http://52.202.244.43/addfilesfromfolder   // to add folder both GET and API
// http://52.202.244.43/reorderfiles   // to reorder files in the DB both GET and API for random order for users each time
// http://52.202.244.43/encryptfilenames // to encrypt filenames with expiration both GET and API
// http://52.202.244.43/media/7xyAilja...   // to server mp3s

// Key words:
//golang mp3 server, logic to encrypt filenames, add new mp3/wav files from system folder location, reorder files for random listening  - works!!!!

//creates new uuid using golang logic
//takes the substring of a string that has separators '/' or '.'
//able to add firestore firebase record using custom id. 
//Also see logic for old server that was doing all including static web server - File: ../../goServer/src/serverMultiFoldersAndHome9000.go (old)

//This new server, does not server index.html etc, but is able to server mp3 after making sure the filename has expiration valid.
//It also encrypts file names - schedule this say every day.
//It also re orders files - for random listening
//It also uploads file names from the system folders

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
	"io/ioutil"
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
const BASEFOLDER = "/home/cloud9/solfeggio/goServer/public/"

const FOLDER_SELECT_DIRECTIVE = 1 //See 1 2 or 3 below

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

func main() {

	router := httprouter.New()
	// decrypt and stream mp3
	router.GET("/media/:encryptedfilename", checkPassPhraseThenStreamMediaHandler) //The audio SRC takes the URL and uses GET? - Leaving both here
	router.GET("/media", needParametersError)                                      // if no file name is provide, show error/information, so the folders and not displayed
	router.POST("/media/:encryptedfilename", checkPassPhraseThenStreamMediaHandler)
	router.POST("/media", needParametersError) //so the folders and not displayed, if no file name is provide, show error/information

	// encrypt and send encrpted name, maybe used by hourly process? This to add the encrypted name to say a encrypted filename column to the firestore database
	// this helps us hide the real MP3 filename and also keep the validity of the url/filename for short period (1 day, etc), 
	// so users do not bypass the app and use and bookmark the MP3 direct url
	router.GET("/encryptfilenames/:encryptedfilename", noParameterExpectedError)
	router.GET("/encryptfilenames", encryptDBFileNamesWithExpirationLogic)
	router.POST("/encryptfilenames/:encryptedfilename", noParameterExpectedError)
	router.POST("/encryptfilenames", encryptDBFileNamesWithExpirationLogic)

	// if the go program also need to server the image or thumbnail of the song icon.
	router.GET("/image/:imagefilename", serveImageHandler)
	router.GET("/image", needParametersError)
	router.POST("/image/:imagefilename", serveImageHandler)
	router.POST("/image", needParametersError)

	// initial load - adds all the files from a folder to database collection
	router.GET("/addfilesfromfolder/:name", noParameterExpectedError)
	router.GET("/addfilesfromfolder", readFolderAddDBRecords)
	router.POST("/addfilesfromfolder/:name", noParameterExpectedError)
	router.POST("/addfilesfromfolder", readFolderAddDBRecords)
	
	// reorder records/documents randomly- so users get different flavor each time
	router.GET("/reorderfiles/:input", noParameterExpectedError)
	router.GET("/reorderfiles", reorderDocumentsRandomly)
	router.POST("/reorderfiles/:input", noParameterExpectedError)
	router.POST("/reorderfiles", reorderDocumentsRandomly)

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
		// } else if r.URL.Path == "/SolfeggioImageByGerdAltmannFromPixabay.jpg" {
		// 	http.ServeFile(w, r, BASEFOLDER+"SolfeggioImageByGerdAltmannFromPixabay.jpg")
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

	fmt.Println("Golang web server starting at port: ", PORT)

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

func checkPassPhraseThenStreamMediaHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	(w).Header().Set("Access-Control-Allow-Origin", "*")
    (w).Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
    (w).Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
    
	encryptedfilename := ps.ByName("encryptedfilename")
	fmt.Println("In checkPassPhraseThenStreamMediaHandler...")
	log.Println(encryptedfilename)

	myReplacer := strings.NewReplacer("tyiCyaB", "/") // the '/' was replaced earlier so it will not mess the URL, now restoring it before decrypt
	encryptedfilename = myReplacer.Replace(encryptedfilename)
	currentfilestring, err2 := b64.StdEncoding.DecodeString(encryptedfilename)

	if err2 != nil { // if the above is not base64, or any other error, file not found etc
		w.Header().Set("Content-Type", "image/jpeg") // <-- set the content-type header
		http.ServeFile(w, r, BASEFOLDER+"404Multi.jpg")
		fmt.Println("error:", err2)
		return
	}

	//Change the hardcoded paraphase value by say today's date, so same url may not be used for mp3 permanently
	//#TODO Also, remove the GET and use only POST to deliver song, so urls to songs are not bookmarked, bypassing the apps.
	filename := decrypt(currentfilestring, "addsomepassword")
	filenameandPath := BASEFOLDER + string(filename)
    // fmt.Println("Decrypted filename and Path = ", filenameandPath)

	if string(filename) == "MY_URL_FILENAME_EXPIRED" || string(filename) == "MY_URL_INCORRECT_LENGTH" {
		// w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		// w.WriteHeader(http.StatusNotFound) // StatusNotFound = 404
		// w.Write([]byte("Resource Invalid/expired."))
		w.Header().Set("Content-Type", "image/jpeg") // <-- set the content-type header
		//http.ServeFile(w, r, BASEFOLDER+"404Multi.jpg")
		http.ServeFile(w, r, BASEFOLDER+"expired.gif")
	} else {
		w.Header().Set("Content-Type", "audio/mpeg") // the golang header for streaming audio file
		http.ServeFile(w, r, filenameandPath)        // the golang is now actually streaming audio file.
		logMediaPlayed(w,r,ps,string(filename))
		log.Println("playing:",filenameandPath)
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
	
	_, _, err = client.Collection("solfeggio_prod_activity").Add(context.Background(), map[string]interface{}{
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

func decrypt(data []byte, passphrase string) []byte {
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
		//The remaining is the filename!, check to see the time received is less than current time, if less pass plaintext back, else error.
		//timepart := plaintext[0:10]
		currenttime := strconv.FormatInt(time.Now().UTC().Unix(), 10)
		timevaliduptopart := "0000000000"
		if len(plaintext) > 12 {
			timevaliduptopart = string(plaintext[0:10]) // get the first 10 bytes that have the timestamp this was created + XYZ (24) Hours
			filenamepart := plaintext[10:]
			plaintext = filenamepart
		}
		//End- check for URL/filename expiry feature
	
	   ///////////// TESTING ALWAYS RETURN media mp3
	   log.Println("playing - NO CHECKING expired condition");
		if timevaliduptopart > currenttime || 1==1 {
			return plaintext
		} else {
			return []byte("MY_URL_FILENAME_EXPIRED")
		}

	} else {
		return []byte("MY_URL_INCORRECT_LENGTH")
	}

}

func serveImageHandler(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	imagefilename := ps.ByName("imagefilename")
    fmt.Println(imagefilename)  
	w.Header().Set("Content-Type", "image/jpeg") // <-- set the content-type header
	//http.ServeFile(w, r, BASEFOLDER+"radio.jpg")	
	http.ServeFile(w, r, BASEFOLDER+imagefilename)		
}

func encryptDBFileNamesWithExpirationLogic(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {

	//dumpHTTPRequest(r) // To debug your HTTP requests, import the net/http/httputil package and pass *http.Request
	fmt.Println("in encryptFileNameWithExpiration...")

	//encryptedfilename := ps.ByName("encryptedfilename") // no filename is sent - we are doing all files on firestore table/encrypted column
	// Use a service account
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

	// First grab the values sent by client Code
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		panic(err)
	}
	//log.Println(string(body))
	var t receivedStruct
	err = json.Unmarshal(body, &t)
	if err != nil {
		//panic(err)
		fmt.Println("Issue with the JSON object: ", err)
	}
	//log.Println(t)
	///////////log.Println("t.Songseq = ", t.Songseq)
	// songseq, err := strconv.Atoi(t.Songseq) // convert string to int
	// if err != nil {
	// 	//panic(err)
	// 	songseq = 0 //defaulting to 0
	// 	fmt.Println("string to number convert issue or JSON not sent by client or POST not used: ", err)
	// }

	//log.Println("Method received: ", r.Method)

	if r.Method == "OPTIONS" {
		log.Println("preflight or what?")
		w.Header().Set("Content-Type", "application/json")
		w.Write(nil)
	} else if r.Method == "POST" || r.Method == "GET" {
   fmt.Fprint(w, "Encrypting filenames\n")
   fmt.Println("Encrypting filenames\n")
   iter := client.Collection("solfeggio_prod_master").Documents(context.Background())
   for {
        doc, err := iter.Next()
        if err == iterator.Done {
                break
        }
        if err != nil {
                //return err
                fmt.Println(err)
        }
        filename := doc.Data()["filenamewithpath"]
        fmt.Println(filename)
        // go SDK could not get the id of the document in select - so as a workaround added the key "workaroundgoid" as a field to the collection with value = id of doc.
        id := doc.Data()["workaroundgoid"]
        // The error was" cannot convert filename (type interface {}) to type []byte: need type assertion, fixed by replacing filename by fmt.Sprint(filename)
		ciphercurrentfile := encrypt([]byte(fmt.Sprint(filename)), "addsomepassword")
        ciphercurrentfilestring := b64.StdEncoding.EncodeToString(ciphercurrentfile)
        myReplacer := strings.NewReplacer("/", "tyiCyaB") // replace "/"" by predefined string say tyiCyaB
        ciphercurrentfilestring = myReplacer.Replace(ciphercurrentfilestring)
        efilename := ciphercurrentfilestring

        fmt.Println(filename)        
        fmt.Println(efilename)        
        
        
        // check this out - may not need the workaroundgoid field <<<<<<<<<<<<<<<<<<<<<<<<<<<<
        // https://godoc.org/cloud.google.com/go/firestore
        //_, err = ca.Update(ctx, []firestore.Update{{Path: "capital", Value: "Sacramento"}})
        
					//updating the efilename after encrypting
					_, err = client.Collection("solfeggio_prod_master").Doc(fmt.Sprint(id)).Set(context.Background(), map[string]interface{}{
					        "efilename": efilename,
					}, firestore.MergeAll)
					
					if err != nil {
					        // Handle any errors in an appropriate way, such as returning them.
					        log.Printf("An error has occurred: %s", err)
					}
  }
 }
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

   iter := client.Collection("solfeggio_prod_master").Documents(context.Background())
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
					_, err = client.Collection("solfeggio_prod_master").Doc(fmt.Sprint(id)).Set(context.Background(), map[string]interface{}{
					        "orderseq": randomnumber,
					}, firestore.MergeAll)
					
					if err != nil {
					        // Handle any errors in an appropriate way, such as returning them.
					        log.Printf("An error has occurred: %s", err)
					}
  }
}	

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
			FOLDER2BADDED := "media/solfeggio/*"   // the * is important - it means read ALL file names from the folder and store list in the variable.
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
				
				//get the final folder, use it as the default for the music classification (Example: meditation, solfeggio, etc - refine later if required)
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
				
				//encrypt filenames
				ciphercurrentfile := encrypt([]byte(fmt.Sprint(filenamewithtoplevelfolders)), "addsomepassword")
		        ciphercurrentfilestring := b64.StdEncoding.EncodeToString(ciphercurrentfile)
		        myReplacer := strings.NewReplacer("/", "tyiCyaB") // replace "/"" by predefined string say tyiCyaB
		        ciphercurrentfilestring = myReplacer.Replace(ciphercurrentfilestring)

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
				 _, err = client.Collection("solfeggio_prod_master").Doc(uuid).Set(context.Background(), map[string]interface{}{
					"workaroundgoid": uuid,
					"filename": justfilename,
					"filenamewithpath": filenamewithtoplevelfolders,
					"efilename": ciphercurrentfilestring,
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
				// _, _, err = client.Collection("solfeggio_prod_master").Add(context.Background(), map[string]interface{}{
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

