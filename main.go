package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"whatsapp-sender/config"
	"whatsapp-sender/constants"
	"whatsapp-sender/db"
	"whatsapp-sender/handler"
	"whatsapp-sender/handler/templates"
	"whatsapp-sender/models"
	"whatsapp-sender/redis"
	"whatsapp-sender/utils"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// create a websocket server

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type LogLevel string

const (
	Info  LogLevel = "info"
	Error LogLevel = "error"
	Debug LogLevel = "debug"
)

type Log struct {
	Level     LogLevel `json:"level" bson:"level"`
	Message   string   `json:"message" bson:"message"`
	Timestamp string   `json:"timestamp" bson:"timestamp"`

	Service string `json:"service" bson:"service"`
}

func wsHandler(c *gin.Context) {
	w := c.Writer
	r := c.Request

	service := r.URL.Query().Get("service")

	if service == "" {
		c.JSON(400, gin.H{
			"message": "Service is required",
		})
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		c.JSON(400, gin.H{
			"message": "Error in upgrading connection " + err.Error(),
		})
		return
	}

	for {
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println(err)
			return
		}

		if string(msg) == "ping" {
			conn.WriteMessage(msgType, []byte("pong"))
			continue
		}

		if string(msg) == "close" {
			conn.Close()
			return
		}

		log.Println("Message received: ", string(msg))
	}
}

func main() {
	// Create a new instance of the server

	// Load environment variables
	// config.LoadEnv()

	// gin.SetMode(gin.ReleaseMode)

	db.ConnectDB()
	redis.ConnectToRedis()
	models.CreateConstantModelIndex()
	// models.CreateLayoutConstant()

	db.RegisterModels(constants.WhatsappTemplateCollection, constants.MessageLogCollection)

	go utils.QueueProcessing()

	// gin.SetMode(gin.ReleaseMode)
	server := gin.Default()

	gin.SetMode(gin.ReleaseMode)

	// Enable CORS
	server.Use(cors.New(cors.Config{
		AllowOriginFunc: func(origin string) bool {
			return true
		},

		AllowMethods: []string{"*"},

		// appversion,authorization,content-type,service,
		AllowHeaders:     []string{"*"},
		AllowCredentials: true,
	}))

	server.GET("/", handler.Home)
	server.GET("/app-logger-ws", wsHandler)

	templateRoutes := server.Group("/templates")
	otpRoutes := server.Group("/otp")
	constantRoutes := server.Group("/constants")

	otpRoutes.POST("/send", handler.SendOTP)
	otpRoutes.POST("/validate", handler.ValidateOtp)

	server.GET("/message-logs", handler.ListMessageLog)

	templateRoutes.GET("", templates.GetTemplateList)
	templateRoutes.POST("", templates.CreateTemplate)
	templateRoutes.DELETE(":id", templates.DeleteTemplate)

	templateRoutes.POST("/send-message", func(ctx *gin.Context) {

		var data struct {
			ID        string            `json:"id" binding:"required"`
			Variables map[string]string `json:"variables" binding:"required"`
			Phone     []string          `json:"phone" binding:"required"`
		}

		err := ctx.ShouldBind(&data)

		if err != nil {
			ctx.JSON(400, gin.H{
				"message": "Error in parsing data",
				"error":   err.Error(),
			})
			return
		}

		if len(data.Phone) == 0 {
			ctx.JSON(400, gin.H{
				"message": "Invalid data",
				"error":   "Phone number is required",
			})
			return
		}

		template, err := models.GetTemplateByName(data.ID)

		if err != nil {
			ctx.JSON(500, gin.H{
				"error": err.Error(),
			})
			return
		}

		errorList := models.ValidateVariables(template, data.Variables)

		if len(errorList) != 0 {
			ctx.JSON(400, gin.H{
				"message": "Invalid data",
				"error":   strings.Join(errorList, "\n"),
			})
			return
		}

		result := models.GenerateWhatsappMessage(template, data.Variables)

		utils.QueueMessage(result, data.Phone)

		ctx.JSON(200, gin.H{
			"message": "Message in queue... will be sent soon",
		})

	})

	constantRoutes.GET("/bank-assign-types", handler.GetBankAssignTypes)
	constantRoutes.GET("/all", handler.GetAllConstants)

	constantRoutes.POST("/", handler.CreateConstant)
	constantRoutes.POST("/bank-assign-types", handler.CreateBankConstant)
	constantRoutes.DELETE("/:id", handler.DeleteConstant)

	// Start the server on port 8080
	port := fmt.Sprintf(":%s", config.GetEnvConfig().PORT)
	server.Run(port)
}
