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
	"github.com/aws/aws-sdk-go/service/dynamodb/expression"
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
	Viewers  string `json:"viewers"`
}

func Quiz(email string, svc *dynamodb.DynamoDB) QuestionType {

	filt := expression.Not(expression.Name("viewers").Contains(email + "|"))

	expr, _ := expression.NewBuilder().WithFilter(filt).Build()

	input := &dynamodb.ScanInput{
		TableName:                 aws.String(TABLE_QUIZ),
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		FilterExpression:          expr.Filter(),
	}

	result, err := svc.Scan(input)

	if err != nil {
		fmt.Println("Error verifying item")
		fmt.Println(err.Error())
	}

	if len(result.Items) == 0 {
		fmt.Println("empty list")
		os.Exit(0)
	}

	preItem := result.Items[0]

	var item QuestionType
	err = dynamodbattribute.UnmarshalMap(preItem, &item)

	if err != nil {
		log.Fatalf("Got error unmarshalling: %s", err)
	}
	return item
}

func QuizAnswer(svc *dynamodb.DynamoDB) string {
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

	return item.Answer
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

	s3Item := Quiz(email, svcDynamodb).Question

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

func saveEmail(email string, quiz QuestionType, svc *dynamodb.DynamoDB) {
	params := &dynamodb.PutItemInput{
		Item: map[string]*dynamodb.AttributeValue{
			"question": {
				S: aws.String(quiz.Question),
			},
			"id": {
				S: aws.String(quiz.Id),
			},
			"answer": {
				S: aws.String(quiz.Answer),
			},
			"viewers": {
				S: aws.String(quiz.Viewers + email + "|"),
			},
		},
		TableName: aws.String(TABLE_QUIZ),
	}
	_, err := svc.PutItem(params)

	if err != nil {
		fmt.Println(err.Error())
	}
}

func checkAnswer(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query()["email"][0]
	id := r.URL.Query()["id"][0]
	ans := r.URL.Query()["ans"][0]
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

	data := make(map[string]bool)

	answer := QuizAnswer(svcDynamodb)

	defer r.Body.Close()

	data["correct"] = (ans == answer)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(data)

	// add email

	quiz := Quiz(email, svcDynamodb)

	saveEmail(email, quiz, svcDynamodb)

}
