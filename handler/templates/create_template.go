package templates

import (
	"whatsapp-sender/models"

	"github.com/gin-gonic/gin"
)

func CreateTemplate(ctx *gin.Context) {
	var payload models.WhatsappTemplate

	payload.SetDefault()

	err := ctx.ShouldBind(&payload)

	if err != nil {
		ctx.JSON(400, gin.H{
			"error": err.Error(),
		})
		return
	}

	err = payload.Validate()

	if err != nil {
		ctx.JSON(400, gin.H{
			"error": err.Error(),
		})
		return
	}

	err = payload.Create()

	if err != nil {
		ctx.JSON(500, gin.H{
			"error": err.Error(),
		})
		return
	}

	ctx.JSON(200, gin.H{
		"payload": payload,
	})

}
