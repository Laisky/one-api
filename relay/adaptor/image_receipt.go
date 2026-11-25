package adaptor

// ImageReceiptAcceptedKey marks a verified rendered-image receipt independently
// of downstream delivery, which must not decide whether paid work occurred.
const ImageReceiptAcceptedKey = "relay.image_receipt_accepted"

// ImageReceiptRejectedKey marks a provider response without an accepted image.
// The image billing path refunds this explicitly rejected attempt.
const ImageReceiptRejectedKey = "relay.image_receipt_rejected"

// ImageStreamReceiptKey stores the ImageStreamReceipt an image handler observed
// while relaying a streamed (text/event-stream) Images API response, so the
// image billing path can judge whether the summed usage covers the request.
const ImageStreamReceiptKey = "relay.image_stream_receipt"

// ImageStreamReceipt summarizes the billing evidence of one relayed Images API
// event stream. Completed counts final-image events (`*.completed`) and
// Receipts those of them that carried a usage object; Partials counts streamed
// partial images (`*.partial_image`). Failed reports an error event, and
// Interrupted a stream that ended without a clean end of body or with an event
// whose type or usage could not be read, so its usage may omit paid work.
type ImageStreamReceipt struct {
	Completed   int
	Receipts    int
	Partials    int
	Failed      bool
	Interrupted bool
}
