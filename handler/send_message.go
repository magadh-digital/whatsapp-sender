package handler

// POST /templates/send-message: the route handler. The decision (preset for named variables, Meta's definition
// for positional values) lives in handler/templates/send_message.go, which stays free of the Redis queue so it
// can be unit-tested.

import (
	"strings"

	"notify/handler/metatemplates"
	"notify/handler/templates"
	"notify/models"
	"notify/utils"

	"github.com/gin-gonic/gin"
)

func SendTemplateMessage(ctx *gin.Context) {
	var req templates.SendRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(400, gin.H{"message": "Error in parsing data", "error": err.Error()})
		return
	}
	if len(req.Phone) == 0 {
		ctx.JSON(400, gin.H{"message": "Invalid data", "error": "Phone number is required"})
		return
	}
	msg, status, errText := templates.Plan(req, models.GetTemplateByName, metatemplates.Resolve)
	if errText != "" {
		ctx.JSON(status, gin.H{"message": "Invalid data", "error": errText})
		return
	}
	ref := strings.TrimSpace(req.Ref)
	utils.QueueMessageRef(msg, req.Phone, ref)
	ctx.JSON(200, gin.H{
		"message":  "Message in queue... will be sent soon",
		"queued":   len(req.Phone),
		"template": msg.Name,
		"language": msg.Language.Code,
		"ref":      ref,
	})
}
