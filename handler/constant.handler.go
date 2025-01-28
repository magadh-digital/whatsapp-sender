package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"whatsapp-sender/constants"
	"whatsapp-sender/models"
	"whatsapp-sender/redis"

	"github.com/gin-gonic/gin"
	"github.com/spidey52/go-helper/helper"
	"go.mongodb.org/mongo-driver/bson"
)

func clearCache() {
	redis.DefaultRedisClient().Del(context.Background(), redis.RedisKeys.ConstantCache)
}

func CreateConstant(ctx *gin.Context) {

	type ConstantCreateDto struct {
		Title     string `json:"title" binding:"required"`
		Category  string `json:"category" binding:"required"`
		SortValue int    `json:"sort_value" binding:"required"`
	}

	var body ConstantCreateDto

	err := ctx.ShouldBindJSON(&body)

	if err != nil {
		ctx.JSON(400, gin.H{
			"error": err.Error(),
		})
		return
	}

	constant := models.Constants{
		Title:     body.Title,
		Category:  body.Category,
		SortValue: body.SortValue,
		CreatedAt: time.Now(),
	}

	_, err = models.ConstantsModel().InsertOne(context.Background(), constant)

	if err != nil {
		ctx.JSON(500, gin.H{
			"error": err.Error(),
		})
		return
	}

	clearCache()

	ctx.JSON(200, gin.H{
		"message": "Constant created successfully",
	})
}

func CreateBankConstant(ctx *gin.Context) {

	type BankAssignTypeCreateDto struct {
		Title                string `json:"title" binding:"required"`
		Value                string `json:"value" binding:"required"`
		IsPoVisible          bool   `json:"is_po_visible" binding:"required"`
		IsBeneficiaryVisible bool   `json:"is_beneficiary_visible" binding:"required"`
		Credit               bool   `json:"credit" binding:"required"`
		Debit                bool   `json:"debit" binding:"required"`
		Description          string `json:"description" binding:"required"`
	}

	var body BankAssignTypeCreateDto

	err := ctx.ShouldBindJSON(&body)

	if err != nil {
		ctx.JSON(400, gin.H{
			"error": err.Error(),
		})
		return
	}

	bankAssignType := models.BankAssignType{
		Title:                body.Title,
		Value:                body.Value,
		IsPoVisible:          body.IsPoVisible,
		IsBeneficiaryVisible: body.IsBeneficiaryVisible,
		Credit:               body.Credit,
		Debit:                body.Debit,
		Description:          body.Description,
	}

	_, err = models.BankAssignTypeModel().InsertOne(context.Background(), bankAssignType)

	if err != nil {
		ctx.JSON(500, gin.H{
			"error": err.Error(),
		})
		return
	}

	clearCache()

	ctx.JSON(200, gin.H{
		"message": "Bank Assign Type created successfully",
	})
}

func GetAllConstants(ctx *gin.Context) {
	// Get all constants
	cached := redis.DefaultRedisClient().Get(context.Background(), redis.RedisKeys.ConstantCache).Val()

	// fmt.Println("Data fetched from cache", cached)

	if cached != "" {
		var parsedResult map[string]interface{}
		err := json.Unmarshal([]byte(cached), &parsedResult)

		fmt.Println("Data fetched from cache")

		if err != nil {
			ctx.JSON(500, gin.H{
				"error": err.Error(),
			})
			return
		}

		ctx.JSON(200, parsedResult)

		return
	}

	bankAssignTypes, err := helper.FindMany[models.BankAssignType](models.BankAssignTypeModel(), nil)

	if err != nil {
		ctx.JSON(500, gin.H{
			"error": err.Error(),
		})
		return
	}

	cursor, err := models.ConstantsModel().Aggregate(context.Background(), []bson.M{
		{
			"$sort": bson.M{
				"sort_value": 1,
			},
		},
		{
			"$group": bson.M{
				"_id": "$category",
				"data": bson.M{
					"$push": "$title",
				},
			},
		},
	})

	if err != nil {
		ctx.JSON(500, gin.H{
			"error": err.Error(),
		})
		return
	}

	type AggregatedData struct {
		ID   string   `json:"id" bson:"_id"`
		Data []string `json:"data" bson:"data"`
	}

	var groupedData []AggregatedData

	cursor.All(context.Background(), &groupedData)

	groupedDataMap := make(map[string]map[string]string)
	groupedDataArray := make(map[string][]string)

	for _, data := range groupedData {
		key := strings.ToLower(data.ID)
		groupedDataMap[key] = make(map[string]string)
		groupedDataArray[key+"_list"] = data.Data

		for _, value := range data.Data {
			groupedDataMap[key][value] = value
		}
	}

	result := gin.H{
		"pdf_report":        constants.PdfReportConstantsList,
		"bank_assign_types": bankAssignTypes,
	}

	for key, value := range groupedDataMap {
		result[key] = value
	}

	for key, value := range groupedDataArray {
		result[key] = value
	}

	result["app_pages"] = result["app_pages_list"]

	strData, _ := json.Marshal(result)

	_, err = redis.DefaultRedisClient().Set(context.TODO(), redis.RedisKeys.ConstantCache, string(strData), time.Minute).Result()

	if err != nil {
		fmt.Println("Error in caching data", err)
		ctx.JSON(500, gin.H{
			"error": err.Error(),
		})
		return
	}

	ctx.JSON(200, result)

}

func GetBankAssignTypes(ctx *gin.Context) {
	// Get all constants
	bankAssignTypes, err := helper.FindMany[models.BankAssignType](models.BankAssignTypeModel(), nil)

	if err != nil {
		ctx.JSON(500, gin.H{
			"error": err.Error(),
		})
		return
	}

	ctx.JSON(200, bankAssignTypes)
}

func DeleteConstant(ctx *gin.Context) {
	id := ctx.Param("id")

	_, err := models.ConstantsModel().DeleteOne(context.Background(), bson.M{"value": id})

	if err != nil {
		ctx.JSON(500, gin.H{
			"error": err.Error(),
		})
		return
	}

	clearCache()

	ctx.JSON(200, gin.H{
		"message": "Constant deleted successfully",
	})
}
