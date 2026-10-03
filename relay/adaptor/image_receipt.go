package adaptor

// ImageReceiptAcceptedKey marks a verified rendered-image receipt independently
// of downstream delivery, which must not decide whether paid work occurred.
const ImageReceiptAcceptedKey = "relay.image_receipt_accepted"

// ImageReceiptRejectedKey marks a provider response without an accepted image.
// The image billing path refunds this explicitly rejected attempt.
const ImageReceiptRejectedKey = "relay.image_receipt_rejected"
