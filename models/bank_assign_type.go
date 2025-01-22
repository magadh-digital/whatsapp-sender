package models

import (
	"whatsapp-sender/db"

	"go.mongodb.org/mongo-driver/mongo"
)

type BankAssignType struct {
	Title                string `json:"title" bson:"title"`
	Value                string `json:"value" bson:"value"`
	IsPoVisible          bool   `json:"is_po_visible" bson:"is_po_visible"`
	IsBeneficiaryVisible bool   `json:"is_beneficiary_visible" bson:"is_beneficiary_visible"`
	Credit               bool   `json:"credit" bson:"credit"`
	Debit                bool   `json:"debit" bson:"debit"`
	Description          string `json:"description" bson:"description"`
}

func BankAssignTypeModel() *mongo.Collection {
	return db.DB.Collection("bank_assign_types")
}
