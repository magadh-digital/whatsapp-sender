package utils

import "go.mongodb.org/mongo-driver/mongo/options"

func GetFingOptions(skip int64, limit int64, sortField string, sortOrder int) options.FindOptions {

	option := options.FindOptions{
		Skip:  &skip,
		Limit: &limit,
		Sort:  map[string]int{sortField: sortOrder},
	}

	return option
}
