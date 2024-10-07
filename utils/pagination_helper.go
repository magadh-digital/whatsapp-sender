package utils

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type PaginationData struct {
	Skip      int64
	Limit     int64
	Search    string
	StartDate time.Time
	EndDate   time.Time
	Date      time.Time
}

// default value for
func ConvertStringToInt64(value string, defaultValue int64) int64 {

	result, err := strconv.ParseInt(value, 10, 64)

	if err != nil {
		return defaultValue
	}

	return result
}

func detectTimeFormat(str string) string {

	layouts := []string{
		time.RFC3339,     // 2006-01-02T15:04:05Z07:00
		time.RFC3339Nano, // 2006-01-02T15:04:05.999999999Z07:00
		time.RFC1123,     // Mon, 02 Jan 2006 15:04:05 MST
		time.RFC1123Z,    // Mon, 02 Jan 2006 15:04:05 -0700
		time.ANSIC,       // Mon Jan 2 15:04:05 2006
		time.DateTime,
		time.DateOnly,
		time.TimeOnly,
		time.Kitchen,  // Custom layout: 3:04PM
		time.UnixDate, // Custom layout: Mon Jan _2 15:04:05 MST 2006
		time.RubyDate, // Custom layout: Mon Jan 02 15:04:05 -0700 2006
	}

	for _, layout := range layouts {
		if _, err := time.Parse(layout, str); err == nil {
			return layout
		}
	}

	return time.DateTime
}

func ConvertStringToTime(value string) time.Time {

	if value == "" {
		return time.Time{}
	}

	format := detectTimeFormat(value)

	result, err := time.ParseInLocation(format, value, time.Local)

	if err != nil {
		return time.Time{}
	}
	return result
}

func GetPaginationData(c *gin.Context) PaginationData {

	var paginationData PaginationData

	page := c.Query("page")
	limit := c.Query("limit")
	search := c.Query("search")
	startDate := c.Query("startDate")
	endDate := c.Query("endDate")
	date := c.Query("date")

	paginationData.Limit = ConvertStringToInt64(limit, 20)
	paginationData.Skip = (ConvertStringToInt64(page, 1)) * paginationData.Limit
	paginationData.Search = search

	paginationData.StartDate = ConvertStringToTime(startDate)
	paginationData.EndDate = ConvertStringToTime(endDate)
	paginationData.Date = ConvertStringToTime(date)

	// how to print timezone of time

	fmt.Println("Timezone of time is ", paginationData.StartDate.Location(), " and time is ", paginationData.StartDate)

	// check if time value is zero then set it to end of the day in endDate

	if paginationData.EndDate.Hour() == 0 && paginationData.EndDate.Minute() == 0 && paginationData.EndDate.Second() == 0 {
		paginationData.EndDate = paginationData.EndDate.Add(23*time.Hour + 59*time.Minute + 59*time.Second + 999*time.Millisecond)
	}

	return paginationData
}
