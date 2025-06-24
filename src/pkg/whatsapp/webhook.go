package whatsapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	pkgError "github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/error"
	"github.com/sirupsen/logrus"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// forwardEventToWebhook is a generic helper function to forward any event payload to webhook URLs
func forwardEventToWebhook(eventType string, payload map[string]interface{}) error {
	logrus.Infof("Forwarding %s to webhook: %v", eventType, config.WhatsappWebhook)

	for _, url := range config.WhatsappWebhook {
		if err := submitWebhook(payload, url); err != nil {
			return err
		}
	}

	logrus.Infof("%s forwarded to webhook", strings.Title(eventType))
	return nil
}

// forwardToWebhook is a helper function to forward event to webhook url
func forwardToWebhook(evt *events.Message) error {
	payload, err := createPayload(evt)
	if err != nil {
		return err
	}
	return forwardEventToWebhook("event", payload)
}

func createPayload(evt *events.Message) (map[string]interface{}, error) {
	message := buildEventMessage(evt)
	waReaction := buildEventReaction(evt)
	forwarded := buildForwarded(evt)

	body := make(map[string]interface{})

	// Add event type identifier for consistency with receipt webhooks
	body["event_type"] = "message"

	if from := evt.Info.SourceString(); from != "" {
		body["from"] = from
		body["from_me"] = isFromMySelf(from)
	}
	if message.ID != "" {
		body["message"] = message
	}
	if pushname := evt.Info.PushName; pushname != "" {
		body["pushname"] = pushname
	}
	if waReaction.Message != "" {
		body["reaction"] = waReaction
	}
	if evt.IsViewOnce {
		body["view_once"] = evt.IsViewOnce
	}
	if forwarded {
		body["forwarded"] = forwarded
	}
	if timestamp := evt.Info.Timestamp.Format(time.RFC3339); timestamp != "" {
		body["timestamp"] = timestamp
	}

	if audioMedia := evt.Message.GetAudioMessage(); audioMedia != nil {
		path, err := ExtractMedia(config.PathMedia, audioMedia)
		if err != nil {
			logrus.Errorf("Failed to download audio from %s: %v", evt.Info.SourceString(), err)
			return nil, pkgError.WebhookError(fmt.Sprintf("Failed to download audio: %v", err))
		}
		body["audio"] = path
	}

	if contactMessage := evt.Message.GetContactMessage(); contactMessage != nil {
		body["contact"] = contactMessage
	}

	if documentMedia := evt.Message.GetDocumentMessage(); documentMedia != nil {
		path, err := ExtractMedia(config.PathMedia, documentMedia)
		if err != nil {
			logrus.Errorf("Failed to download document from %s: %v", evt.Info.SourceString(), err)
			return nil, pkgError.WebhookError(fmt.Sprintf("Failed to download document: %v", err))
		}
		body["document"] = path
	}

	if imageMedia := evt.Message.GetImageMessage(); imageMedia != nil {
		path, err := ExtractMedia(config.PathMedia, imageMedia)
		if err != nil {
			logrus.Errorf("Failed to download image from %s: %v", evt.Info.SourceString(), err)
			return nil, pkgError.WebhookError(fmt.Sprintf("Failed to download image: %v", err))
		}
		body["image"] = path
	}

	if listMessage := evt.Message.GetListMessage(); listMessage != nil {
		body["list"] = listMessage
	}

	if liveLocationMessage := evt.Message.GetLiveLocationMessage(); liveLocationMessage != nil {
		body["live_location"] = liveLocationMessage
	}

	if locationMessage := evt.Message.GetLocationMessage(); locationMessage != nil {
		body["location"] = locationMessage
	}

	if orderMessage := evt.Message.GetOrderMessage(); orderMessage != nil {
		body["order"] = orderMessage
	}

	if stickerMedia := evt.Message.GetStickerMessage(); stickerMedia != nil {
		path, err := ExtractMedia(config.PathMedia, stickerMedia)
		if err != nil {
			logrus.Errorf("Failed to download sticker from %s: %v", evt.Info.SourceString(), err)
			return nil, pkgError.WebhookError(fmt.Sprintf("Failed to download sticker: %v", err))
		}
		body["sticker"] = path
	}

	if videoMedia := evt.Message.GetVideoMessage(); videoMedia != nil {
		path, err := ExtractMedia(config.PathMedia, videoMedia)
		if err != nil {
			logrus.Errorf("Failed to download video from %s: %v", evt.Info.SourceString(), err)
			return nil, pkgError.WebhookError(fmt.Sprintf("Failed to download video: %v", err))
		}
		body["video"] = path
	}

	return body, nil
}

func submitWebhook(payload map[string]interface{}, url string) error {
	client := &http.Client{Timeout: 10 * time.Second}

	postBody, err := json.Marshal(payload)
	if err != nil {
		return pkgError.WebhookError(fmt.Sprintf("Failed to marshal body: %v", err))
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(postBody))
	if err != nil {
		return pkgError.WebhookError(fmt.Sprintf("error when create http object %v", err))
	}

	secretKey := []byte(config.WhatsappWebhookSecret)
	signature, err := getMessageDigestOrSignature(postBody, secretKey)
	if err != nil {
		return pkgError.WebhookError(fmt.Sprintf("error when create signature %v", err))
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", fmt.Sprintf("sha256=%s", signature))

	var attempt int
	var maxAttempts = 5
	var sleepDuration = 1 * time.Second

	for attempt = 0; attempt < maxAttempts; attempt++ {
		if _, err = client.Do(req); err == nil {
			logrus.Infof("Successfully submitted webhook on attempt %d", attempt+1)
			return nil
		}
		logrus.Warnf("Attempt %d to submit webhook failed: %v", attempt+1, err)
		time.Sleep(sleepDuration)
		sleepDuration *= 2
	}

	return pkgError.WebhookError(fmt.Sprintf("error when submit webhook after %d attempts: %v", attempt, err))
}

// forwardReceiptToWebhook is a helper function to forward receipt event to webhook url
func forwardReceiptToWebhook(evt *events.Receipt) error {
	payload, err := createReceiptPayload(evt)
	if err != nil {
		return err
	}
	return forwardEventToWebhook("receipt event", payload)
}

func createReceiptPayload(evt *events.Receipt) (map[string]interface{}, error) {
	body := make(map[string]interface{})

	// Add event type identifier
	body["event_type"] = "receipt"

	// Add message IDs that were read/delivered
	if len(evt.MessageIDs) > 0 {
		body["message_ids"] = evt.MessageIDs
	}

	// Add sender information
	if sender := evt.SourceString(); sender != "" {
		body["sender"] = sender
	}

	// Add receipt type (delivered/read)
	var receiptType string
	switch evt.Type {
	case types.ReceiptTypeRead, types.ReceiptTypeReadSelf:
		receiptType = "read"
	case types.ReceiptTypeDelivered:
		receiptType = "delivered"
	default:
		receiptType = "unknown"
	}
	body["type"] = receiptType

	// Add timestamp
	if timestamp := evt.Timestamp.Format(time.RFC3339); timestamp != "" {
		body["timestamp"] = timestamp
	}

	return body, nil
}
