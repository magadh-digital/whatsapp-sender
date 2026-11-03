package models

// A campaign is one template message to a list of phones, asked for by a service (the console's broadcast through
// mds-orders) and owned here from then on: an approved template is queued at once; a message written as a new
// template waits in Mongo until Meta approves it (the campaigns loop checks), then is queued. Every queued message
// carries the campaign's ref on its log row, so /notify-logs?ref= shows the campaign's delivery.

import (
	"context"
	"errors"
	"time"

	"notify/db"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const CampaignCollection = "campaigns"

const (
	CampaignAwaiting  = "awaiting_approval"
	CampaignQueued    = "queued"
	CampaignRejected  = "rejected"
	CampaignCancelled = "cancelled"
	CampaignFailed    = "failed"
)

// CampaignCompose is the message written for a campaign, which became a Meta template of the campaign's name.
type CampaignCompose struct {
	Body     string `bson:"body" json:"body"`
	Footer   string `bson:"footer,omitempty" json:"footer,omitempty"`
	Category string `bson:"category" json:"category"`
}

type CampaignBy struct {
	ID   string `bson:"id" json:"id"`
	Name string `bson:"name" json:"name"`
}

type Campaign struct {
	ID primitive.ObjectID `bson:"_id,omitempty" json:"-"`
	// the caller's id for this campaign (a console broadcast's job id); unique; the `ref` on every log row
	Ref      string `bson:"ref" json:"ref"`
	Name     string `bson:"name" json:"name"`
	Language string `bson:"language" json:"language"`
	// the body's {{1}}..{{n}} values, the header value / media link, URL-button values by 1-based position
	Params  []string          `bson:"params" json:"params"`
	Header  string            `bson:"header,omitempty" json:"header,omitempty"`
	Buttons map[string]string `bson:"buttons,omitempty" json:"buttons,omitempty"`
	// 10-digit phones; not answered to callers (the count is)
	Phones     []string `bson:"phones" json:"-"`
	PhoneCount int      `bson:"phone_count" json:"phones"`
	// awaiting_approval | queued | rejected | cancelled | failed
	Status string `bson:"status" json:"status"`
	// Meta's template status at the last look (PENDING, APPROVED, REJECTED…), "" when never looked up
	MetaStatus     string `bson:"meta_status,omitempty" json:"meta_status,omitempty"`
	RejectedReason string `bson:"rejected_reason,omitempty" json:"rejected_reason,omitempty"`
	// the last check's or queueing's problem, "" when none
	Error   string           `bson:"error,omitempty" json:"error,omitempty"`
	Compose *CampaignCompose `bson:"compose,omitempty" json:"compose,omitempty"`
	// a composed message whose text matched a template already approved on Meta: sent with it, no new template
	ReusedTemplate bool       `bson:"reused_template,omitempty" json:"reused_template,omitempty"`
	By             CampaignBy `bson:"by" json:"by"`
	CreatedAt      time.Time  `bson:"created_at" json:"created_at"`
	UpdatedAt      time.Time  `bson:"updated_at" json:"updated_at"`
	CheckedAt      *time.Time `bson:"checked_at,omitempty" json:"checked_at,omitempty"`
	QueuedAt       *time.Time `bson:"queued_at,omitempty" json:"queued_at,omitempty"`
	// phones pushed to the send queue
	Queued int `bson:"queued" json:"queued"`
}

var ErrNoStore = errors.New("campaign store not connected")

func campaigns() *mongo.Collection {
	if db.DB == nil {
		return nil
	}
	return db.DB.Collection(CampaignCollection)
}

// EnsureCampaignIndexes: ref unique, and the loop's status + created_at scan.
func EnsureCampaignIndexes() {
	c := campaigns()
	if c == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, _ = c.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "ref", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "created_at", Value: -1}}},
	})
}

func (c *Campaign) Insert() error {
	coll := campaigns()
	if coll == nil {
		return ErrNoStore
	}
	c.PhoneCount = len(c.Phones)
	res, err := coll.InsertOne(context.Background(), c)
	if err != nil {
		return err
	}
	c.ID = res.InsertedID.(primitive.ObjectID)
	return nil
}

// Save writes the campaign back by ref.
func (c *Campaign) Save() error {
	coll := campaigns()
	if coll == nil {
		return ErrNoStore
	}
	c.PhoneCount = len(c.Phones)
	_, err := coll.ReplaceOne(context.Background(), bson.M{"ref": c.Ref}, c)
	return err
}

// GetCampaign: nil, nil when there is none with that ref.
func GetCampaign(ref string) (*Campaign, error) {
	coll := campaigns()
	if coll == nil {
		return nil, ErrNoStore
	}
	var c Campaign
	err := coll.FindOne(context.Background(), bson.M{"ref": ref}).Decode(&c)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListCampaigns: newest first; status "" = every status; refs non-empty = only those refs.
func ListCampaigns(status string, refs []string, limit int64) ([]Campaign, error) {
	coll := campaigns()
	if coll == nil {
		return nil, ErrNoStore
	}
	q := bson.M{}
	if status != "" {
		q["status"] = status
	}
	if len(refs) > 0 {
		q["ref"] = bson.M{"$in": refs}
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	cur, err := coll.Find(context.Background(), q, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(limit))
	if err != nil {
		return nil, err
	}
	defer cur.Close(context.Background())
	out := []Campaign{}
	for cur.Next(context.Background()) {
		var c Campaign
		if cur.Decode(&c) == nil {
			out = append(out, c)
		}
	}
	return out, nil
}
