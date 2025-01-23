package constants

const (
	// collections
	WhatsappTemplateCollection = "whatsapp_templates"
	MessageLogCollection       = "message_logs"
)

type PdfReportConstants struct {
	Title          string `json:"report_title" bson:"report_title"`
	Value          string `json:"report_value" bson:"report_value"`
	DefaultChecked bool   `json:"default_checked" bson:"default_checked"`
}

var PdfReportConstantsList = []PdfReportConstants{
	{Title: "summary", Value: "SUMMARY", DefaultChecked: false},
	{Title: "kata report", Value: "KATA_REPORT", DefaultChecked: true},
	{Title: "order report", Value: "ORDER_REPORT", DefaultChecked: true},
	{Title: "gift report", Value: "GIFT_REPORT", DefaultChecked: true},
	{Title: "parking report", Value: "PARKING_REPORT", DefaultChecked: true},
	{Title: "bank report", Value: "BANK_REPORT", DefaultChecked: true},
	{Title: "raw material report", Value: "RAW_MATERIAL_REPORT", DefaultChecked: true},
	{Title: "bank all", Value: "BANK_ALL", DefaultChecked: false},
	{Title: "bank debit", Value: "BANK_DEBIT", DefaultChecked: true},
	{Title: "loading user", Value: "REPORT_WITH_LOADING_USER", DefaultChecked: true},
	{Title: "loading supervisor", Value: "REPORT_WITH_LOADING_SUPERVISOR", DefaultChecked: true},
}
