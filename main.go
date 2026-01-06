package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type commandlineArgs struct {
	host     string
	username string
	password string
}

func parseArgs() commandlineArgs {
	a := commandlineArgs{}

	flag.StringVar(&a.host, "h", "", "host of the cm1200 modem")
	flag.StringVar(&a.username, "u", "", "http username")
	flag.StringVar(&a.password, "p", "", "http password")

	flag.Parse()

	if a.host == "" {
		slog.Error("-h is required")
		os.Exit(-1)
	}

	if a.username == "" {
		slog.Error("-u is required")
		os.Exit(-1)
	}

	if a.password == "" {
		slog.Error("-p is required")
		os.Exit(-1)
	}
	return a
}

func getTagLine(body string, lineNum int) []string {
	lines := strings.Split(body, "\n")

	content := strings.Split(lines[lineNum], "'")[1]

	return strings.Split(content, "|")
}

func main() {
	args := parseArgs()
	for {
		// Grab xsrf token
		resp, err := http.DefaultClient.Get("http://" + args.host)
		if err != nil {
			slog.Error("Error sending xsrf request", "error", err)
			os.Exit(-1)
		}

		xsrfCookie := resp.Header.Get("Set-Cookie")

		authRequest, err := http.NewRequest("GET", "http://"+args.host+"/DocsisStatus.htm", nil)
		authRaw := []byte(args.username + ":" + args.password)
		authn := make([]byte, base64.StdEncoding.EncodedLen(len(authRaw)))

		base64.StdEncoding.Encode(authn, authRaw)

		if err != nil {
			slog.Error("Error building auth request", "error", err)
			os.Exit(-1)
		}

		authRequest.Header.Add("Authorization", "Basic "+string(authn))
		authNCookies, err := http.ParseCookie(xsrfCookie)
		if err != nil {
			slog.Error("Error parsing authn cookie", "error", err)
			os.Exit(-1)
		}
		for _, cookie := range authNCookies {
			authRequest.AddCookie(cookie)
		}

		resp, err = http.DefaultTransport.RoundTrip(authRequest)
		if err != nil {
			slog.Error("Error send auth request", "error", err)
			os.Exit(-1)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			slog.Error("Error reading body of response", "error", err)
			os.Exit(-1)
		}

		tags := getTagLine(string(body), 176)
		dsChannelTags := getTagLine(string(body), 306)
		usChannelTags := getTagLine(string(body), 253)

		dsLocked := 0
		dsUnlocked := 0
		dsUncorrectable := 0

		for i := 1; i < len(dsChannelTags)-9; i += 9 {
			switch dsChannelTags[i+1] {
			case "Locked":
				dsLocked++
			case "Not Locked":
				dsUnlocked++
			}
			uncorrectable, err := strconv.ParseInt(dsChannelTags[i+8], 10, 0)
			if err != nil {
				panic("Shouldn't happen")
			}
			dsUncorrectable += int(uncorrectable)
		}

		usLocked := 0
		usUnlocked := 0
		for i := 0; i < len(usChannelTags)-7; i += 7 {
			switch usChannelTags[i+2] {
			case "Locked":
				usLocked++
			case "Not Locked":
				usUnlocked++
			}
		}
		fmt.Printf("%s: ", tags[10])
		if tags[12] != "0" {
			fmt.Printf("\x1b[31mDownStream Partial(locked(%d) not locked(%d), uncorr(%d))\033[m, ", dsLocked, dsUnlocked, dsUncorrectable)
		} else {
			fmt.Printf("\x1b[32mDownStream Up     (locked(%d) not locked(%d), uncorr(%d))\033[m, ", dsLocked, dsUnlocked, dsUncorrectable)
		}

		if tags[13] != "0" {
			fmt.Printf("\x1b[31mUpStream Partial(locked(%d) not locked(%d))\033[m\n", usLocked, usUnlocked)
		} else {
			fmt.Printf("\x1b[32mUpstream Up     (locked(%d) not locked(%d))\033[m\n", usLocked, usUnlocked)
		}

		time.Sleep(2 * time.Second)
	}
}
