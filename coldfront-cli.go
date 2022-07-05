package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	// "net"
	"net/http"
	// "net/url"
	// "os"
	"io/ioutil"
	"log"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	cv "github.com/nirasan/go-oauth-pkce-code-verifier"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"
)

type AllocationData []struct {
	ID                        int `json:"id"`
	ProjectID                 int `json:"project_id"`
	AllPublicAttributesAsList []struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		Value string `json:"value"`
		Usage string `json:"usage"`
	} `json:"all_public_attributes_as_list"`
	AttributesValues, AttributesNames, AttributesUsage map[int]string
}

const (
	clientID     = "XXXX"
	clientSecret = "XXXX"
	authDomain   = "http://localhost:9000/users-api/o"
	redirectURL  = authDomain + "/showcode"
	CF_ENDPOINT  = "http://localhost:9000/users-api/slurm/rysy-gpu"
)

func login() {
	// initialize the code verifier
	var CodeVerifier, _ = cv.CreateCodeVerifier()

	// Create code_challenge with S256 method
	codeChallenge := CodeVerifier.CodeChallengeS256()

	// construct the authorization URL (with Auth0 as the authorization provider)
	authorizationURL := fmt.Sprintf(
		"%s/authorize?"+
			"response_type=code&client_id=%s"+
			"&code_challenge=%s"+
			"&code_challenge_method=S256&redirect_uri=%s",
		authDomain, clientID, codeChallenge, redirectURL)

	fmt.Printf("Visit the following URL for the auth dialog (ctrl_click is often enough):\n %v \n", authorizationURL)

	fmt.Printf("Paste the token obtained from web-page:\n")
	var code string
	if _, err := fmt.Scan(&code); err != nil {
		log.Fatal(err)
	}

	codeVerifier := CodeVerifier.String()

	// ctx := context.Background()
	// conf := &oauth2.Config{
	// 	ClientID:     clientID,
	// 	ClientSecret: clientSecret,
	// 	Scopes:       []string{"READ"},
	// 	Endpoint: oauth2.Endpoint{
	// 		AuthURL:  authDomain + "/authorize",
	// 		TokenURL: authDomain + "/token/",
	// 	},
	// }
	// params := AuthCodeOptionParam{
	// 	codeVerifier,
	// }

	url := authDomain + "/token/"
	data := fmt.Sprintf(
		"grant_type=authorization_code"+
			"&client_id=%s"+
			"&code_verifier=%s"+
			"&code=%s"+
			"&redirect_uri=%s"+
			"&client_secret=%s",
		clientID, codeVerifier, code, redirectURL, clientSecret)
	payload := strings.NewReader(data)

	// create the request and execute it
	req, _ := http.NewRequest("POST", url, payload)
	req.Header.Add("content-type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	// process the response
	defer res.Body.Close()
	var responseData map[string]interface{}
	body, _ := ioutil.ReadAll(res.Body)

	// unmarshal the json into a string map
	err = json.Unmarshal(body, &responseData)
	if err != nil {
		log.Fatal(err)
	}
	for k, v := range responseData {
		fmt.Println(k, "value is", v)
	}

	viper.Set("AccessToken", responseData["access_token"].(string))
	viper.Set("ExpiresIn", time.Now().Add(time.Duration(responseData["expires_in"].(float64))).Format(time.RFC1123))
	viper.Set("RefreshToken", responseData["refresh_token"].(string))
	viper.Set("Scope", responseData["scope"].(string))
	viper.Set("TokenType", responseData["token_type"].(string))

	err = viper.WriteConfigAs("auth.json")
	//_, err = config.WriteConfigFile("auth.json", token)
	if err != nil {
		log.Fatal(err)
	}

	// client := conf.Client(ctx, responseData)
	// client.Get("http://localhost:9000/users-api/slurm/topola")
}

func main() {
	ctx := context.Background()
	viper.AddConfigPath(".")
	viper.SetConfigName("auth")

	conf := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       []string{"READ"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  authDomain + "/authorize",
			TokenURL: authDomain + "/token/",
		},
	}
	var token oauth2.Token
	var savedToken *oauth2.Token

	if err := viper.ReadInConfig(); err != nil {
		login()
		if err := viper.ReadInConfig(); err != nil {
			log.Fatal("Unable to authorize.")
		}
	}

	token.AccessToken = viper.Get("AccessToken").(string)
	token.RefreshToken = viper.Get("RefreshToken").(string)
	token.Expiry, _ = time.Parse(time.RFC1123, viper.Get("Expiresin").(string))
	token.TokenType = viper.Get("TokenType").(string)

	tokenSource := conf.TokenSource(oauth2.NoContext, &token)

	client := oauth2.NewClient(ctx, tokenSource)

	savedToken, _ = tokenSource.Token()

	res, err := client.Get(CF_ENDPOINT)
	if err != nil {
		log.Fatal("Unauthorized - remove auth.json file to reauthorize.")
	}
	viper.Set("AccessToken", savedToken.AccessToken)
	viper.Set("Expiresin", savedToken.Expiry.Format(time.RFC1123))
	viper.Set("RefreshToken", savedToken.RefreshToken)
	viper.Set("TokenType", savedToken.TokenType)
	err = viper.WriteConfigAs("auth.json")
	//_, err = config.WriteConfigFile("auth.json", token)
	if err != nil {
		log.Fatal(err)
	}

	body, err := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode > 299 {
		log.Fatalf("Response failed with status code: %d and\nbody: %s\n", res.StatusCode, body)
	}
	if err != nil {
		log.Fatal(err)
	}

	//fmt.Println(string(body))

	var allAllocations AllocationData

	err = json.Unmarshal(body, &allAllocations)
	if err != nil {
		log.Fatal(err)
	}

	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.AppendHeader(table.Row{"Project", "Allocation ID", "CPUh", "", "GPUh", "", "Account"})

	t.AppendRows([]table.Row{
		table.Row{" ", " ", "Available", "Used", "Available", "Used", ""},
	})
	t.AppendSeparator()
	for _, allocation := range allAllocations {
		// fmt.Println("Allocation ID", allocation.ID)
		// fmt.Println("Allocation attributes: ")
		AttributesValues := map[int]string{}
		AttributesNames := map[int]string{}
		AttributesUsage := map[int]string{}

		for _, v := range allocation.AllPublicAttributesAsList {
			// fmt.Println("    ", v.ID, "    ", v.Name, ": ", v.Value, " / ", v.Usage)

			AttributesNames[v.ID] = v.Name
			AttributesValues[v.ID] = v.Value
			AttributesUsage[v.ID] = v.Usage

		}

		allocation.AttributesValues = AttributesValues
		allocation.AttributesNames = AttributesNames
		allocation.AttributesUsage = AttributesUsage

		t.AppendRows([]table.Row{
			{"https://granty.icm.edu.pl/p/" + fmt.Sprint(allocation.ProjectID),
				allocation.ID,
				allocation.AttributesValues[3],
				allocation.AttributesUsage[3],
				allocation.AttributesValues[26],
				allocation.AttributesUsage[26],
				allocation.AttributesValues[14],
			},
		})

	}

	// t.AppendSeparator()
	// t.AppendFooter(table.Row{"", "", "Total", 10000})
	t.Render()

}
