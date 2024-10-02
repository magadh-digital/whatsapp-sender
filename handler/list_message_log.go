package handler

import (
	"whatsapp-sender/models"
	"whatsapp-sender/utils"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
)

func ListMessageLog(ctx *gin.Context) {

	paginationData := utils.GetPaginationData(ctx)

	skip := paginationData.Skip
	limit := paginationData.Limit
	search := paginationData.Search
	startDate := paginationData.StartDate
	endDate := paginationData.EndDate

	status := ctx.Query("status")
	template := ctx.Query("template")

	filterQuery := bson.M{}

	if status != "" {
		filterQuery["status"] = status
	}

	if template != "" {
		filterQuery["template"] = template
	}

	if search != "" {

		filterQuery["$or"] = []bson.M{
			{"phone": bson.M{"$regex": search, "$options": "i"}},
			{"template": bson.M{"$regex": search, "$options": "i"}},
		}

	}

	if !startDate.IsZero() && !endDate.IsZero() {
		filterQuery["created_at"] = bson.M{
			"$gte": startDate,
			"$lte": endDate,
		}
	}

	option := utils.GetFingOptions(skip, limit, "created_at", -1)

	logs, err := models.GetMessageLogs(filterQuery, option)
	count := models.CountMessageLogs(filterQuery)

	if err != nil {
		ctx.JSON(500, gin.H{
			"error":   err.Error(),
			"message": "Internal server error",
		})
		return
	}

	ctx.JSON(200, gin.H{
		"logs":  logs,
		"count": count,
	})
}
