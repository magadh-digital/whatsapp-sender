package models

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
	"whatsapp-sender/db"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type WhatsappTemplate struct {
	ID primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`

	Title       string   `json:"title" bson:"title"`
	Description string   `json:"description" bson:"description"`
	Status      string   `json:"status" bson:"status"`
	Variables   []string `json:"variables" bson:"-"`

	CreatedAt time.Time `json:"created_at" bson:"created_at"`

	Name string `json:"name" binding:"required" bson:"name"`

	Language struct {
		Code string `json:"code"`
	} `json:"language" bson:"language"`

	Components []struct {
		Type       string `json:"type" binding:"required"`
		Parameters []struct {
			Type string `json:"type" binding:"required"`
			Text string `json:"text,omitempty" bson:"text,omitempty"`

			Image *struct {
				Link string `json:"link,omitempty" bson:"link,omitempty"`
			} `json:"image,omitempty" bson:"image,omitempty"`
		} `json:"parameters"`
	} `json:"components"`
}

func (w *WhatsappTemplate) SetDefault() WhatsappTemplate {

	if w.Language.Code == "" {
		w.Language.Code = "en"
	}

	if w.Status == "" {
		w.Status = "ACTIVE"
	}

	if w.CreatedAt.IsZero() {
		w.CreatedAt = time.Now()
	}

	if w.Variables == nil {
		w.Variables = make([]string, 0)
	}

	return *w
}

func (w *WhatsappTemplate) Validate() error {

	if w.Components == nil {
		return fmt.Errorf("components is null")
	}

	if len(w.Components) == 0 {
		return fmt.Errorf("components is empty")
	}

	for _, component := range w.Components {
		if component.Type == "" {
			return fmt.Errorf("component type is empty")
		}

		if component.Parameters == nil {
			return fmt.Errorf("parameters is null")
		}

		if len(component.Parameters) == 0 {
			return fmt.Errorf("parameters is empty")
		}

		fmt.Println("component.Parameters", component.Parameters)
		for _, parameter := range component.Parameters {

			if parameter.Type == "" {
				return fmt.Errorf("parameter type is empty")
			}

			if parameter.Type == "text" {
				if parameter.Text == "" {
					return fmt.Errorf("parameter text is empty")
				}
			}

			if parameter.Type == "image" {
				if parameter.Image == nil {
					return fmt.Errorf("image is null")
				}
				// if parameter.Image.Link == "" {
				// 	return fmt.Errorf("image link is empty")
				// }
			}

		}

	}

	return nil
}

func (w *WhatsappTemplate) Create() error {
	_, err := db.WhatsappTemplateModel.InsertOne(context.Background(), w)

	if err != nil {
		return err

	}

	return nil

}
func (w *WhatsappTemplate) Update() error {
	objID, err := primitive.ObjectIDFromHex(w.ID.Hex())

	if err != nil {
		return err
	}

	_, err = db.WhatsappTemplateModel.UpdateOne(context.Background(), gin.H{"_id": objID}, gin.H{"$set": w})

	if err != nil {
		return err
	}

	return nil
}

func ListTemplates() ([]WhatsappTemplate, error) {

	cursor, err := db.WhatsappTemplateModel.Find(context.Background(), gin.H{})

	if err != nil {
		fmt.Println("error 1", err)
		return nil, err
	}

	var templates []WhatsappTemplate = make([]WhatsappTemplate, 0)

	err = cursor.All(context.Background(), &templates)

	for i := range templates {
		templates[i].ExtractVariables()
	}

	if err != nil {
		fmt.Println("error 2", err)
		return nil, err
	}

	return templates, nil
}

func GetTemplate(id string) (*WhatsappTemplate, error) {

	objID, err := primitive.ObjectIDFromHex(id)

	if err != nil {
		return nil, err
	}

	var template WhatsappTemplate

	err = db.WhatsappTemplateModel.FindOne(context.Background(), gin.H{"_id": objID}).Decode(&template)

	template.ExtractVariables()

	if err != nil {
		return nil, err
	}

	return &template, nil
}

func GetTemplateByName(name string) (*WhatsappTemplate, error) {

	var template WhatsappTemplate

	err := db.WhatsappTemplateModel.FindOne(context.Background(), gin.H{"name": name}).Decode(&template)

	if err != nil {
		return nil, err
	}

	return &template, nil
}

func DeleteTemplate(id string) error {

	objID, err := primitive.ObjectIDFromHex(id)

	if err != nil {
		return err
	}

	_, err = db.WhatsappTemplateModel.DeleteOne(context.Background(), gin.H{"_id": objID})

	if err != nil {
		return err
	}

	return nil
}

func GenerateWhatsappMessage(template *WhatsappTemplate, data map[string]string) *WhatsappTemplate {

	for i := range template.Components {
		for j := range template.Components[i].Parameters {
			if template.Components[i].Parameters[j].Type == "text" {
				val := data[template.Components[i].Parameters[j].Text]
				template.Components[i].Parameters[j].Text = val
			}

			if template.Components[i].Parameters[j].Type == "image" {
				val := data[template.Components[i].Parameters[j].Image.Link]
				template.Components[i].Parameters[j].Image.Link = val
			}
		}
	}

	jsonTemplate, _ := json.Marshal(template)

	fmt.Println("jsonTemplate", string(jsonTemplate))

	return template
}

func ValidateVariables(template *WhatsappTemplate, data map[string]string) []string {

	errorMessages := make([]string, 0)

	variables := template.ExtractVariables()

	for i := range variables {
		if data[variables[i]] == "" {
			message := fmt.Sprintf("variable %s is empty", variables[i])
			errorMessages = append(errorMessages, message)
		}
	}

	return errorMessages
}

func (w *WhatsappTemplate) ExtractVariables() []string {
	var variables []string = make([]string, 0)

	for i := range w.Components {
		for j := range w.Components[i].Parameters {
			if w.Components[i].Parameters[j].Type == "text" {
				variables = append(variables, w.Components[i].Parameters[j].Text)
			}

			if w.Components[i].Parameters[j].Type == "image" {
				variables = append(variables, w.Components[i].Parameters[j].Image.Link)
			}
		}
	}

	w.Variables = variables

	return variables
}

var WhatsappRequest = http.Request{}

func SendMessage(template *WhatsappTemplate, phone string) error {
	// send message to phone

	body := map[string]interface{}{

		"messaging_product": "whatsapp",
		"to":                "91" + phone,
		"type":              "template",

		"template": map[string]interface{}{
			"name": template.Name,

			"language":   template.Language,
			"components": template.Components,
		},
	}

	// io reader body

	jsonBody, _ := json.Marshal(body)

	ioBody := bytes.NewReader(jsonBody)

	token := os.Getenv("WHATSAPP_AUTH")
	url := os.Getenv("WHATSAPP_URL")

	req, err := http.NewRequest("POST", url, ioBody)

	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	client := &http.Client{}

	resp, err := client.Do(req)

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)

	if err != nil {
		return err
	}

	var payload map[string]interface{}

	err = json.Unmarshal([]byte(string(respBody)), &payload)

	if err != nil {
		fmt.Println("error in unmarshalling", err)
		return err
	}

	if resp.StatusCode != 200 {
		NewMessageLog(template.Name, payload, phone, MessageLogStatusFailed, bson.M{
			"components": template.Components,
		}).Create()
		return fmt.Errorf(string(respBody))
	}

	NewMessageLog(template.Name, payload, phone, MessageLogStatusSuccess, bson.M{
		"components": template.Components,
	}).Create()

	return nil
}
