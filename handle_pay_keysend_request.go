package main

import (
	"context"

	"github.com/getAlby/nostr-wallet-connect/events"
	"github.com/nbd-wtf/go-nostr"
	"github.com/sirupsen/logrus"
)

func (svc *Service) HandlePayKeysendEvent(ctx context.Context, request *Nip47Request, requestEvent *RequestEvent, app *App, publishResponse func(*Nip47Response, *nostr.Tags)) {

	payParams := &Nip47KeysendParams{}
	resp := svc.decodeNip47Request(request, requestEvent, app, payParams)
	if resp != nil {
		publishResponse(resp, &nostr.Tags{})
		return
	}

	resp = svc.checkPermission(request, requestEvent, app, payParams.Amount)
	if resp != nil {
		publishResponse(resp, &nostr.Tags{})
		return
	}

	payment := Payment{App: *app, RequestEvent: *requestEvent, Amount: uint(payParams.Amount / 1000)}
	err := svc.db.Create(&payment).Error
	if err != nil {
		publishResponse(&Nip47Response{
			ResultType: request.Method,
			Error: &Nip47Error{
				Code:    NIP_47_ERROR_INTERNAL,
				Message: err.Error(),
			},
		}, &nostr.Tags{})
		return
	}

	svc.Logger.WithFields(logrus.Fields{
		"eventId":      requestEvent.NostrId,
		"appId":        app.ID,
		"senderPubkey": payParams.Pubkey,
	}).Info("Sending payment")

	preimage, err := svc.lnClient.SendKeysend(ctx, payParams.Amount, payParams.Pubkey, payParams.Preimage, payParams.TLVRecords)
	if err != nil {
		svc.Logger.WithFields(logrus.Fields{
			"eventId":         requestEvent.NostrId,
			"appId":           app.ID,
			"recipientPubkey": payParams.Pubkey,
		}).Infof("Failed to send payment: %v", err)
		svc.EventLogger.Log(&events.Event{
			Event: "nwc_payment_failed",
			Properties: map[string]interface{}{
				// "error":   fmt.Sprintf("%v", err),
				"keysend": true,
				"amount":  payParams.Amount / 1000,
			},
		})
		publishResponse(&Nip47Response{
			ResultType: request.Method,
			Error: &Nip47Error{
				Code:    NIP_47_ERROR_INTERNAL,
				Message: err.Error(),
			},
		}, &nostr.Tags{})
		return
	}
	payment.Preimage = &preimage
	svc.db.Save(&payment)
	svc.EventLogger.Log(&events.Event{
		Event: "nwc_payment_succeeded",
		Properties: map[string]interface{}{
			"keysend": true,
			"amount":  payParams.Amount / 1000,
		},
	})
	publishResponse(&Nip47Response{
		ResultType: request.Method,
		Result: Nip47PayResponse{
			Preimage: preimage,
		},
	}, &nostr.Tags{})
}
