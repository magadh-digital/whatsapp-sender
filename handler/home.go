package handler

import "github.com/gin-gonic/gin"

func Home(ctx *gin.Context) {
	ctx.JSON(200, gin.H{
		"message": "Hello World",
	})
}
