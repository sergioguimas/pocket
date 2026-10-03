package push

import webpush "github.com/SherClockHolmes/webpush-go"

// WebPush é o Pusher de verdade (github.com/SherClockHolmes/webpush-go,
// validado na fase 0 — FASE-0-SPIKE.md): Urgency high e TTL 3600 (sem isso
// o Android segura o push até sair do Doze, medido no spike) e Subject de
// POCKET_PUSH_SUBJECT (exigido pelo VAPID).
type WebPush struct {
	vapid   VAPIDKeys
	subject string
}

// NewWebPush cria o Pusher a partir das chaves VAPID (CarregarOuGerarVAPID)
// e do subject (config.PushSubject).
func NewWebPush(vapid VAPIDKeys, subject string) *WebPush {
	return &WebPush{vapid: vapid, subject: subject}
}

func (w *WebPush) Enviar(sub Subscription, payload []byte) (status int, err error) {
	resp, err := webpush.SendNotification(payload, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys:     webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth},
	}, &webpush.Options{
		Subscriber:      w.subject,
		VAPIDPublicKey:  w.vapid.Public,
		VAPIDPrivateKey: w.vapid.Private,
		TTL:             3600,
		Urgency:         webpush.UrgencyHigh,
	})
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}
