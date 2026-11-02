// Package campaigns: one template message to a list of phones, owned by this service from the request on.
//
//	POST /campaigns            { ref, phone: [10 digits…],
//	                             template: { name, language, params, header, buttons }   an APPROVED template: queued now
//	                           | compose:  { name, language, category, body, footer } }  a new message: submitted to Meta
//	                                                                                     as a template of that name,
//	                                                                                     queued when Meta approves it
//	GET  /campaigns[?status=&refs=a,b]   newest first
//	GET  /campaigns/:ref                 one campaign (staff session, or a service signed with NOTIFY_STATUS_SECRET)
//	POST /campaigns/:ref/check           look at Meta now instead of waiting for the loop
//	POST /campaigns/:ref/cancel          only while awaiting approval; the template stays on Meta
//
// The loop (`Loop`) checks every awaiting campaign once a minute: approved → the phones are queued with the ref,
// rejected (or gone from Meta) → the campaign says why. Callers see progress on the campaign and the delivery of each
// message in /notify-logs?ref=.
package campaigns

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"notify/handler/metatemplates"
	"notify/middleware"
	"notify/models"

	"github.com/gin-gonic/gin"
)

// QueueFn pushes one send per phone onto the send queue; main sets it to utils.QueueMessageRef (kept out of this
// package so its tests run without Redis).
var QueueFn = func(t *models.WhatsappTemplate, phones []string, ref string) {
	log.Printf("[campaigns] QueueFn not set: %d phones for %s not queued", len(phones), ref)
}

var (
	refRe   = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,80}$`)
	phoneRe = regexp.MustCompile(`^[6-9]\d{9}$`)
	nonDig  = regexp.MustCompile(`\D`)
)

// NormalizePhone: an Indian mobile number as 10 digits (a leading 91 or 0 dropped), "" when it is not one.
func NormalizePhone(p string) string {
	d := nonDig.ReplaceAllString(p, "")
	if len(d) == 12 && strings.HasPrefix(d, "91") {
		d = d[2:]
	} else if len(d) == 11 && strings.HasPrefix(d, "0") {
		d = d[1:]
	}
	if phoneRe.MatchString(d) {
		return d
	}
	return ""
}

type TemplateRef struct {
	Name     string            `json:"name"`
	Language string            `json:"language"`
	Params   []string          `json:"params"`
	Header   string            `json:"header"`
	Buttons  map[string]string `json:"buttons"`
}

type ComposeReq struct {
	Name     string `json:"name"`
	Language string `json:"language"`
	Category string `json:"category"`
	Body     string `json:"body"`
	Footer   string `json:"footer"`
}

type CreateRequest struct {
	Ref      string       `json:"ref" binding:"required"`
	Phone    []string     `json:"phone" binding:"required"`
	Template *TemplateRef `json:"template"`
	Compose  *ComposeReq  `json:"compose"`
}

// Phones: the valid 10-digit numbers, deduped in order, and how many were dropped.
func Phones(raw []string) ([]string, int) {
	seen := map[string]bool{}
	out := make([]string, 0, len(raw))
	dropped := 0
	for _, p := range raw {
		n := NormalizePhone(p)
		if n == "" {
			dropped++
			continue
		}
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	return out, dropped
}

func values(t *TemplateRef) metatemplates.SendValues {
	v := metatemplates.SendValues{Body: t.Params, Header: t.Header, Buttons: map[int]string{}}
	for k, val := range t.Buttons {
		var n int
		if _, err := fmt.Sscanf(k, "%d", &n); err == nil {
			v.Buttons[n] = val
		}
	}
	return v
}

func valuesOf(c *models.Campaign) metatemplates.SendValues {
	v := metatemplates.SendValues{Body: c.Params, Header: c.Header, Buttons: map[int]string{}}
	for k, val := range c.Buttons {
		var n int
		if _, err := fmt.Sscanf(k, "%d", &n); err == nil {
			v.Buttons[n] = val
		}
	}
	return v
}

// Decision is what Apply decided for an awaiting campaign.
type Decision string

const (
	DecideQueue  Decision = "queue"  // approved: queue the phones
	DecideReject Decision = "reject" // rejected or gone: nothing to send
	DecideWait   Decision = "wait"   // still under review
	DecideRetry  Decision = "retry"  // Meta could not be consulted: look again later
)

// Apply records one look at Meta on an awaiting campaign and says what to do; pure, so it is tested without Meta.
func Apply(c *models.Campaign, row *metatemplates.Row, lookupErr string, now time.Time) Decision {
	at := now
	c.CheckedAt = &at
	c.UpdatedAt = now
	if row == nil {
		if strings.HasPrefix(lookupErr, "No template named") {
			c.Status = models.CampaignRejected
			c.RejectedReason = "The template is no longer on Meta"
			c.Error = ""
			return DecideReject
		}
		c.Error = lookupErr
		return DecideRetry
	}
	c.Error = ""
	c.MetaStatus = row.Status
	switch row.Status {
	case "APPROVED":
		return DecideQueue
	case "REJECTED":
		c.Status = models.CampaignRejected
		c.RejectedReason = row.RejectedReason
		if c.RejectedReason == "" {
			c.RejectedReason = "Rejected by Meta"
		}
		return DecideReject
	}
	return DecideWait
}

// queue builds the send from Meta's definition and pushes every phone; the campaign records the outcome.
func queue(c *models.Campaign, row *metatemplates.Row, now time.Time) {
	t, issues := metatemplates.BuildSend(row, valuesOf(c))
	c.UpdatedAt = now
	if len(issues) > 0 {
		c.Status = models.CampaignFailed
		c.Error = strings.Join(issues, "; ")
		return
	}
	QueueFn(t, c.Phones, c.Ref)
	at := now
	c.QueuedAt = &at
	c.Queued = len(c.Phones)
	c.Status = models.CampaignQueued
	c.Error = ""
}

// Process looks at Meta once for an awaiting campaign and acts on it; the campaign is saved. Never panics.
func Process(c *models.Campaign) {
	if c.Status != models.CampaignAwaiting {
		return
	}
	row, msg := metatemplates.Lookup(c.Name, c.Language, true)
	now := time.Now()
	switch Apply(c, row, msg, now) {
	case DecideQueue:
		queue(c, row, now)
		log.Printf("[campaigns] %s (%s) approved: %d phones queued", c.Ref, c.Name, c.Queued)
	case DecideReject:
		log.Printf("[campaigns] %s (%s) rejected: %s", c.Ref, c.Name, c.RejectedReason)
	case DecideRetry:
		log.Printf("[campaigns] %s (%s) check failed: %s", c.Ref, c.Name, c.Error)
	}
	if err := c.Save(); err != nil {
		log.Printf("[campaigns] save %s: %v", c.Ref, err)
	}
}

// Loop checks the awaiting campaigns every `every`; started once from main.
func Loop(every time.Duration) {
	models.EnsureCampaignIndexes()
	time.Sleep(10 * time.Second)
	for {
		list, err := models.ListCampaigns(models.CampaignAwaiting, nil, 200)
		if err != nil {
			log.Printf("[campaigns] list: %v", err)
		}
		for i := range list {
			Process(&list[i])
		}
		time.Sleep(every)
	}
}

// ---------------------------------------------------------------- handlers

func refuse(ctx *gin.Context, status int, msg string) {
	ctx.JSON(status, gin.H{"message": msg})
}

// Create: POST /campaigns.
func Create(ctx *gin.Context) {
	var in CreateRequest
	if err := ctx.ShouldBindJSON(&in); err != nil {
		refuse(ctx, 400, "Unreadable request: "+err.Error())
		return
	}
	in.Ref = strings.TrimSpace(in.Ref)
	if !refRe.MatchString(in.Ref) {
		refuse(ctx, 400, "ref: letters, digits, _ . : - (at most 80)")
		return
	}
	if (in.Template == nil) == (in.Compose == nil) {
		refuse(ctx, 400, "Send exactly one of template or compose")
		return
	}
	phones, dropped := Phones(in.Phone)
	if len(phones) == 0 {
		refuse(ctx, 400, "phone: no valid Indian mobile number")
		return
	}
	if existing, err := models.GetCampaign(in.Ref); err != nil {
		refuse(ctx, 503, "Campaign store: "+err.Error())
		return
	} else if existing != nil {
		ctx.JSON(409, gin.H{"message": "A campaign with this ref exists", "campaign": existing})
		return
	}

	user := middleware.GetSessionUser(ctx)
	by := models.CampaignBy{}
	if user != nil {
		by = models.CampaignBy{ID: user.ID, Name: user.Name}
	}
	now := time.Now()
	c := &models.Campaign{Ref: in.Ref, Phones: phones, Params: []string{}, By: by, CreatedAt: now, UpdatedAt: now}

	if in.Template != nil {
		t := in.Template
		row, msg := metatemplates.Resolve(strings.TrimSpace(t.Name), strings.TrimSpace(t.Language))
		if row == nil {
			refuse(ctx, statusFor(msg), msg)
			return
		}
		c.Name, c.Language, c.Params, c.Header, c.Buttons, c.MetaStatus = row.Name, row.Language, t.Params, t.Header, t.Buttons, row.Status
		if c.Params == nil {
			c.Params = []string{}
		}
		c.Status = models.CampaignAwaiting // queue() sets queued / failed
		queue(c, row, now)
		if c.Status == models.CampaignFailed {
			refuse(ctx, 400, c.Error)
			return
		}
		if err := c.Insert(); err != nil {
			log.Printf("[campaigns] insert %s after queueing: %v", c.Ref, err)
		}
		log.Printf("[campaigns] %s: %s (%s) to %d phones queued by %s", c.Ref, c.Name, c.Language, c.Queued, by.Name)
		ctx.JSON(200, gin.H{"message": "Queued", "campaign": c, "dropped_phones": dropped})
		return
	}

	in2 := in.Compose
	req := &metatemplates.CreateRequest{Name: in2.Name, Category: in2.Category, Language: in2.Language, Body: in2.Body, Footer: in2.Footer, BodyExamples: []string{}}
	payload, issues := metatemplates.BuildPayload(req)
	if len(issues) > 0 {
		ctx.JSON(400, gin.H{"message": strings.Join(issues, "\n"), "issues": issues})
		return
	}
	if strings.Contains(req.Body, "{{") {
		refuse(ctx, 400, "Body: a message written for a campaign has no {{variables}}; use an approved template with values instead")
		return
	}
	res, status, msg, merr := metatemplates.Submit(payload, req, user)
	if status != 200 {
		if merr != nil {
			ctx.JSON(status, gin.H{"message": msg, "meta_error": merr})
			return
		}
		refuse(ctx, status, msg)
		return
	}
	c.Name, c.Language = req.Name, req.Language
	c.Compose = &models.CampaignCompose{Body: req.Body, Footer: req.Footer, Category: req.Category}
	c.Status = models.CampaignAwaiting
	if st, _ := res["status"].(string); st != "" {
		c.MetaStatus = st
	}
	if err := c.Insert(); err != nil {
		refuse(ctx, 503, "Template submitted to Meta but the campaign could not be stored: "+err.Error()+". Create it again with another ref once the store is back; the template "+c.Name+" already exists.")
		return
	}
	log.Printf("[campaigns] %s: new template %s (%s) submitted for %d phones by %s, meta %s", c.Ref, c.Name, c.Language, len(c.Phones), by.Name, c.MetaStatus)
	// Meta sometimes approves on the spot
	if c.MetaStatus == "APPROVED" {
		Process(c)
	}
	ctx.JSON(202, gin.H{"message": "Template submitted to Meta; the phones are queued once it is approved", "campaign": c, "dropped_phones": dropped})
}

func statusFor(msg string) int {
	switch {
	case strings.Contains(msg, "not configured"), strings.HasPrefix(msg, "Could not reach"), strings.HasPrefix(msg, "Meta refused the template list"):
		return 503
	case strings.HasPrefix(msg, "No template named"), strings.Contains(msg, "has no "):
		return 404
	case strings.Contains(msg, "not APPROVED"):
		return 409
	}
	return 400
}

// List: GET /campaigns[?status=&refs=a,b&limit=]
func List(ctx *gin.Context) {
	refs := []string{}
	for _, r := range strings.Split(ctx.Query("refs"), ",") {
		if r = strings.TrimSpace(r); r != "" {
			refs = append(refs, r)
		}
	}
	var limit int64 = 50
	fmt.Sscanf(ctx.DefaultQuery("limit", "50"), "%d", &limit)
	list, err := models.ListCampaigns(strings.TrimSpace(ctx.Query("status")), refs, limit)
	if err != nil {
		refuse(ctx, 503, "Campaign store: "+err.Error())
		return
	}
	ctx.JSON(200, gin.H{"result": list})
}

// Get: GET /campaigns/:ref
func Get(ctx *gin.Context) {
	c, err := models.GetCampaign(ctx.Param("ref"))
	if err != nil {
		refuse(ctx, 503, "Campaign store: "+err.Error())
		return
	}
	if c == nil {
		refuse(ctx, 404, "No campaign with ref "+ctx.Param("ref"))
		return
	}
	ctx.JSON(200, gin.H{"campaign": c})
}

// Check: POST /campaigns/:ref/check — look at Meta now.
func Check(ctx *gin.Context) {
	c, err := models.GetCampaign(ctx.Param("ref"))
	if err != nil {
		refuse(ctx, 503, "Campaign store: "+err.Error())
		return
	}
	if c == nil {
		refuse(ctx, 404, "No campaign with ref "+ctx.Param("ref"))
		return
	}
	if c.Status != models.CampaignAwaiting {
		ctx.JSON(409, gin.H{"message": "This campaign is " + c.Status + ", not waiting for approval", "campaign": c})
		return
	}
	Process(c)
	ctx.JSON(200, gin.H{"campaign": c})
}

// Cancel: POST /campaigns/:ref/cancel — only while awaiting; the template stays on Meta.
func Cancel(ctx *gin.Context) {
	c, err := models.GetCampaign(ctx.Param("ref"))
	if err != nil {
		refuse(ctx, 503, "Campaign store: "+err.Error())
		return
	}
	if c == nil {
		refuse(ctx, 404, "No campaign with ref "+ctx.Param("ref"))
		return
	}
	if c.Status != models.CampaignAwaiting {
		ctx.JSON(409, gin.H{"message": "This campaign is " + c.Status + "; only one waiting for approval can be cancelled", "campaign": c})
		return
	}
	c.Status = models.CampaignCancelled
	c.UpdatedAt = time.Now()
	if err := c.Save(); err != nil {
		refuse(ctx, 503, "Campaign store: "+err.Error())
		return
	}
	if u := middleware.GetSessionUser(ctx); u != nil {
		log.Printf("[campaigns] %s cancelled by %s (%s)", c.Ref, u.Name, u.ID)
	}
	ctx.JSON(200, gin.H{"message": "Cancelled", "campaign": c})
}
