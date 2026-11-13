package level

// EncodeUnchecked writes m as Encode would, without its checks, so tests can write broken files.
var EncodeUnchecked = encode
