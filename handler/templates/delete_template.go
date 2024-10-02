package  templates

import (
	"whatsapp-sender/models"

	"github.com/gin-gonic/gin"
)

func DeleteTemplate(ctx *gin.Context) {
	id := ctx.Param("id")

	err := models.DeleteTemplate(id)

	if err != nil {
		ctx.JSON(500, gin.H{
			"error": err.Error(),
		})
		return
	}

	ctx.JSON(200, gin.H{
		"message": "deleted",
	})
}
