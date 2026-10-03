// Package push envia Web Push (CONTRATO §3.5, §5) — interface Pusher para
// os testes usarem a Fake (regra da casa: sem rede nos testes) e a
// implementação de verdade usar github.com/SherClockHolmes/webpush-go.
package push

// Pusher envia um payload já serializado para uma inscrição. status é o
// código HTTP devolvido pelo serviço de push (204/201 = ok; 404/410 =
// inscrição morta — quem chama remove o push_sub); err é erro de rede/TLS
// etc., sem status para interpretar.
type Pusher interface {
	Enviar(sub Subscription, payload []byte) (status int, err error)
}

// Subscription é a inscrição push de um dispositivo (schema push_sub,
// campos endpoint/p256dh/auth — os três que o Web Push precisa para
// endereçar e cifrar a mensagem).
type Subscription struct {
	Endpoint string
	P256dh   string
	Auth     string
}
