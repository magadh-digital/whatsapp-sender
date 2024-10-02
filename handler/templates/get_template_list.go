package templates

import (
	"whatsapp-sender/models"

	"github.com/gin-gonic/gin"
)

func GetTemplateList(ctx *gin.Context) {

	templateList, err := models.ListTemplates()

	if err != nil {
		ctx.JSON(500, gin.H{
			"error": err.Error(),
		})
		return
	}

	ctx.JSON(200, gin.H{
		"result": templateList,
	})

}
