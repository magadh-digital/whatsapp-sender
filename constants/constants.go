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

var LayoutConstants = map[string]string{
	"NOTICE":         "NOTICE",
	"MEDIA_CAROSEL":  "MEDIA_CAROSEL",
	"PROFILE_CARD":   "PROFILE_CARD",
	"REPORT_CARD":    "REPORT_CARD",
	"ORDERS_COUNT":   "ORDERS_COUNT",
	"SAUDA_SALE":     "SAUDA_SALE",
	"PRODUCT_OFFER":  "PRODUCT_OFFER",
	"BUCKET_SUMMARY": "BUCKET_SUMMARY",
	"SCHEME":         "SCHEME",
	"BASE_RATE":      "BASE_RATE",
	"MONTHLY_SALE":   "MONTHLY_SALE",
	"MO_RANK":        "MO_RANK",
	"BILLET_CARD":    "BILLET_CARD",
}

var VendorServiceConstants = map[string]string{
	"PAINTING":       "PAINTING",
	"LIGHT_BOARD":    "LIGHT_BOARD",
	"FLEX_BOARD":     "FLEX_BOARD",
	"BOARD_PAINTING": "BOARD_PAINTING",
	"WALL_WRAP":      "WALL_WRAP",
}
