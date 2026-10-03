package adaptor

// AudioReceiptAcceptedKey marks successful provider audio work before client
// delivery. It prevents failover from replaying an already billable operation.
const AudioReceiptAcceptedKey = "relay.audio_receipt_accepted"
