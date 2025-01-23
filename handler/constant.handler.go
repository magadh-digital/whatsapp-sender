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

func GetAllConstants(ctx *gin.Context) {
	// Get all constants
	cached := redis.GetRedisClient().Get(context.Background(), redis.RedisKeys.ConstantCache).Val()

	fmt.Println("Data fetched from cache", cached)

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

	strData, _ := json.Marshal(result)

	val, err := redis.GetRedisClient().Set(context.TODO(), redis.RedisKeys.ConstantCache, string(strData), time.Minute).Result()

	if err != nil {
		fmt.Println("Error in caching data", err)
		ctx.JSON(500, gin.H{
			"error": err.Error(),
		})
		return
	}

	fmt.Println("Data cached", val, "value")

	ctx.JSON(200, result)

}

func InsertHello() {
	// 	{
	//   "BANKING": [
	//     {"group": "BANKING", "page": "STORE", "children": []},
	//     {"group": "BANKING", "page": "PAYMENT_REQUEST", "children": []},
	//     {"group": "BANKING", "page": "BANKING_REPORT", "children": []},
	//     {"group": "BANKING", "page": "PURCHASE_ORDER", "children": []},
	//     {"group": "BANKING", "page": "BANKING_BENIFICIARY", "children": []},
	//     {"group": "BANKING", "page": "RAW_MATERIAL", "children": []}
	//   ],
	//   "MAGADH": [
	//     {"group": "MAGADH", "page": "PARTNER", "children": []},
	//     {"group": "MAGADH", "page": "RATE_CHANGE", "children": []},
	//     {"group": "MAGADH", "page": "ORDER", "children": []},
	//     {"group": "MAGADH", "page": "RATE_IMAGE", "children": []},
	//     {"group": "MAGADH", "page": "MEETING", "children": []},
	//     {"group": "MAGADH", "page": "SAUDA", "children": []},
	//     {"group": "MAGADH", "page": "USER_ORDER", "children": []},
	//     {"group": "MAGADH", "page": "ADD_VISITOR", "children": []},
	//     {"group": "MAGADH", "page": "HELP_DESK", "children": []},
	//   ]
	// }

	vals := []string{
		"BANKING:STORE",
		"BANKING:PAYMENT_REQUEST",
		"BANKING:BANKING_REPORT",
		"BANKING:PURCHASE_ORDER",
		"BANKING:BANKING_BENIFICIARY",
		"BANKING:RAW_MATERIAL",
		"MAGADH:PARTNER",
		"MAGADH:RATE_CHANGE",
		"MAGADH:ORDER",
		"MAGADH:RATE_IMAGE",
		"MAGADH:MEETING",
		"MAGADH:SAUDA",
		"MAGADH:USER_ORDER",
		"MAGADH:ADD_VISITOR",
		"MAGADH:HELP_DESK",
	}

	for idx, val := range vals {

		models.ConstantsModel().InsertOne(context.Background(), models.Constants{
			Title:     val,
			Category:  "APP_PAGES",
			SortValue: idx + 1,
			CreatedAt: time.Now(),
		})

	}

}
