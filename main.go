package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
)

const TABLE_USER = "users-caballero"
const TABLE_QUIZ = "quiz-caballero"
const BUCKET_QUIZ = "quiz-caballero"

func init() {
	err := godotenv.Load(".env")
	if err != nil {
		panic(err)
	}
}

func main() {
	fmt.Println("Quiz")
	r := mux.NewRouter()
	r.HandleFunc("/question", askQuestion)
	r.HandleFunc("/ans", checkAnswer)
	http.ListenAndServe(":8080", r)
}

type Message struct {
	Answer string `json:"answer"`
}

type Item struct {
	Id      string `json:"id"`
	Email   string `json:"email"`
	Credits uint16 `json:"credits"`
}

func InexistingItem(id string, email string, svc *dynamodb.DynamoDB) bool {
	params := &dynamodb.GetItemInput{
		TableName: aws.String(TABLE_USER),
		Key: map[string]*dynamodb.AttributeValue{
			"id": {
				S: aws.String(id),
			},
			"email": {
				S: aws.String(email),
			},
		},
	}
	result, err := svc.GetItem(params)

	if err != nil {
		fmt.Println("Error verifying item")
		fmt.Println(err.Error())
	}
	item := Item{}

	err = dynamodbattribute.UnmarshalMap(result.Item, &item)

	if err != nil {
		panic(fmt.Sprintf("Failed to unmarshal Record, %v", err))
	}

	return (item.Id == "")
}

type QuestionType struct {
	Id       string `json:"id"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

func QuizName(svc *dynamodb.DynamoDB) string {
	params := &dynamodb.ScanInput{
		TableName: aws.String(TABLE_QUIZ),
	}
	result, err := svc.Scan(params)

	if err != nil {
		fmt.Println("Error verifying item")
		fmt.Println(err.Error())
	}

	preItem := result.Items[0]

	var item QuestionType
	err = dynamodbattribute.UnmarshalMap(preItem, &item)

	if err != nil {
		log.Fatalf("Got error unmarshalling: %s", err)
	}

	return item.Question

}

func askQuestion(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query()["email"][0]
	id := r.URL.Query()["id"][0]
	defer r.Body.Close()
	aws_access_key_id := os.Getenv("AccessKeyID")
	aws_secret_access_key := os.Getenv("SecretAccessKey")
	region := os.Getenv("REGION")

	sess, err := session.NewSession(&aws.Config{
		Credentials: credentials.NewStaticCredentials(aws_access_key_id, aws_secret_access_key, ""),
		Region:      &region,
	})

	if err != nil {
		panic(err)
	}

	svcDynamodb := dynamodb.New(sess)

	inexisting := InexistingItem(id, email, svcDynamodb)

	if inexisting {
		fmt.Println("Non-existent user.")
		os.Exit(0)
	}

	downloaderS3 := s3manager.NewDownloader(sess)

	s3Item := QuizName(svcDynamodb)

	file, err := os.Create(s3Item)

	if err != nil {
		panic(err)
	}

	s3Param := &s3.GetObjectInput{
		Bucket: aws.String(BUCKET_QUIZ),
		Key:    aws.String(s3Item),
	}

	numBytes, err := downloaderS3.Download(file, s3Param)

	if err != nil {
		panic(err)
	}

	fmt.Println("size: ", numBytes, " bytes")

	sByte, err := ioutil.ReadAll(file)

	if err != nil {
		panic(err)
	}

	w.Write(sByte)
}

func checkAnswer(w http.ResponseWriter, r *http.Request) {
	res, _ := ioutil.ReadAll(r.Body)

	defer r.Body.Close()

	ans := string(res)
	data := make(map[string]bool)
	data["correct"] = (ans == "answer=ANALYTICS")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(data)
}

/*

{
  "id": {
    "S": "JKLjju8ujghjh454645DFDGD0p"
  },
  "question": {
    "S": "fTew43FPoK09.html"
  },
  "answer": {
    "S": "ANALYTICS"
  }
}

*/
