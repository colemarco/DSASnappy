/*
Cole Marco, Satchel Sevenau
DSA Final Project
Snappy compression Algorithm
*/

package main

//--------------------------------------------------------------//
//-------------------------imports------------------------------//
import (
	"encoding/binary"
	"errors"
	"fmt"
)

//--------------------------------------------------------------//
//-----------------------declarations---------------------------//

var maxBlockSize int = 65536

const (
	tagLiteral                            = 0x00
	tagCopy1                              = 0x01
	tagCopy2                              = 0x02
	tagCopy4                              = 0x03
	decodeErrCodeCorrupt                  = 1
	decodeErrCodeUnsupportedLiteralLength = 2
)

var (
	// ErrCorrupt reports that the input is invalid.
	ErrCorrupt = errors.New("snappy: corrupt input")
	// ErrTooLarge reports that the uncompressed length is too large.
	ErrTooLarge = errors.New("snappy: decoded block is too large")
	// ErrUnsupported reports that the input isn't supported.
	ErrUnsupported = errors.New("snappy: unsupported input")

	errUnsupportedLiteralLength = errors.New("snappy: unsupported literal length")
)

//--------------------------------------------------------------//
//---------------------encode functions-------------------------//

func Encode(destination, source []byte) []byte {
	if byteLength := MaxEncodedLen(len(source)); byteLength < 0 {
		panic("Decoded block is too large!")
		// If destination is not large enough to store encoded data, make it
	} else if len(destination) < byteLength {
		destination = make([]byte, byteLength)
	}

	// snappy starts with Little endian varint of the length of decompressed data
	// also bytesWritten keeps track of where in destination we've written
	bytesWritten := binary.PutUvarint(destination, uint64(len(source)))

	// while source still has data to compress
	for len(source) > 0 {
		// Don't modify source, use temp variable
		temp := source
		source = nil
		// To keep implementation consistent with both snappy stream and block
		// formats, we copy all data in temp that's longer than max block back
		// into source byte array
		if len(temp) > maxBlockSize {
			temp, source = temp[:maxBlockSize], temp[maxBlockSize:]
		}
		// If length is less than 17 (the minimum size of the input to encodeBlock that
		// could be encoded with a copy tag) then encode it with a literal. This is the
		// minimum with respect to the algorithm used by encodeBlock, not a minimum
		// enforced by the file format
		if len(temp) < 17 {
			bytesWritten += emitLiteral(destination[bytesWritten:], temp)
		} else {
			bytesWritten += encodeBlock(destination[bytesWritten:], temp)
		}
	}
	return destination[:bytesWritten]
}

func MaxEncodedLen(sourceLength int) int {
	// convert length of source to unsigned 64 bit int
	length := uint64(sourceLength)
	// if block is too large to encode
	if length > 0xffffffff {
		return -1
	}

	length = 32 + length + length/6
	// same check, but after max possible item blowup (worst case compression)
	if length > 0xffffffff {
		return -1
	}
	//convert back to int
	return int(length)
}

// load functions copied directly from snappy documetation
func load32(b []byte, i int) uint32 {
	b = b[i : i+4 : len(b)] // Help the compiler eliminate bounds checks on the next line.
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

func load64(b []byte, i int) uint64 {
	b = b[i : i+8 : len(b)] // Help the compiler eliminate bounds checks on the next line.
	return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
		uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
}

func emitLiteral(destination, literal []byte) int {
	bytesForLiteralLength, literalLength := 0, uint(len(literal)-1)
	switch {
	// if length is < 60, we can encode it in 1 byte
	case literalLength < 60:
		destination[0] = uint8(literalLength)<<2 | tagLiteral
		bytesForLiteralLength = 1
	// if length is < 256, we can encode it in 2 bytes, and 60 is tag byte for
	// taking an extra byte to store literal data
	case literalLength < 1<<8:
		destination[0] = 60<<2 | tagLiteral
		destination[1] = uint8(literalLength)
		bytesForLiteralLength = 2
	default:
		// otherwise, we can encode it in 3 bytes, and 61 is tag byte for
		// taking 2 extra bytes to store literal data
		destination[0] = 61<<2 | tagLiteral
		destination[1] = uint8(literalLength)
		destination[2] = uint8(literalLength >> 8)
		bytesForLiteralLength = 3
	}
	// We return the amounts of bytes written by the literal
	return bytesForLiteralLength + copy(destination[bytesForLiteralLength:], literal)
}

func emitCopy(destination []byte, offset, length int) int {
	bytesForCopyLength := 0
	for length >= 68 {
		// Write a length 64 copy, encoded with 3 bytes
		destination[bytesForCopyLength+0] = 63<<2 | tagCopy2
		destination[bytesForCopyLength+1] = uint8(offset)
		destination[bytesForCopyLength+2] = uint8(offset >> 8)
		bytesForCopyLength += 3
		length -= 64
	}
	if length > 64 {
		// Write a length 60 copy, encoded as 3 bytes
		destination[bytesForCopyLength+0] = 59<<2 | tagCopy2
		destination[bytesForCopyLength+1] = uint8(offset)
		destination[bytesForCopyLength+2] = uint8(offset >> 8)
		bytesForCopyLength += 3
		length -= 60
	}
	if length >= 12 || offset >= 2048 {
		// Write the remaining copy, encoded as 3 bytes
		destination[bytesForCopyLength+0] = uint8(length-1)<<2 | tagCopy2
		destination[bytesForCopyLength+1] = uint8(offset)
		destination[bytesForCopyLength+2] = uint8(offset >> 8)
		bytesForCopyLength += 3
	}
	// Write final remaining copy, encoded as 2 bytes
	destination[bytesForCopyLength+0] = uint8(offset>>8)<<5 | uint8(length-2)<<2 | tagCopy1
	destination[bytesForCopyLength+1] = uint8(offset)
	// Want to return amount of bytes written to destination
	return bytesForCopyLength + 2
}

// function hash copied directly from snappy documentation, we couldn't figure
// out what was so special about the u * 0x1e35a7bd term.
func hash(u, shift uint32) uint32 {
	return (u * 0x1e35a7bd) >> shift
}

func encodeBlock(destination, source []byte) (d int) {
	// Intiialize hash table with size ranging from 1<<8 to 1<<14 inclusive
	const (
		maxTableSize = 1 << 14
		tableMask    = maxTableSize - 1
	)
	shift := uint32(32 - 8)
	for tableSize := 1 << 8; tableSize < maxTableSize && tableSize < len(source); tableSize *= 2 {
		shift--
	}
	var table [maxTableSize]uint16

	// When to stop looking for copies
	sLimit := len(source) - 15

	// nextEmit is where in src the next emitLiteral should start from.
	nextEmit := 0

	// The encoded form must start with a literal, as there are no previous
	// bytes to copy, so we start looking for hash matches at s == 1.
	s := 1
	nextHash := hash(load32(source, s), shift)

	for {
		// Heuristic match skipping: If 32 bytes are scanned with no matches
		// found, start looking only at every other byte. If 32 more bytes are
		// scanned (or skipped), look at every third byte, etc.. When a match
		// is found, immediately go back to looking at every byte.
		//
		// The "skip" variable keeps track of how many bytes there are since
		// the last match; dividing it by 32 (ie. right-shifting by five) gives
		// the number of bytes to move ahead for each iteration.
		skip := 32

		nextS := s
		candidate := 0
		for {
			s = nextS
			// number of bytes to move ahead for each iteration
			bytesBetweenHashLookups := skip >> 5
			// go to next location for iteration
			nextS = s + bytesBetweenHashLookups
			skip += bytesBetweenHashLookups
			// if next would bring us over search limit, skip the rest of iteration
			if nextS > sLimit {
				// goto label on line
				goto emitRemainder
			}
			// candidate is now a possible match
			candidate = int(table[nextHash&tableMask])
			// sets table location just checked to current match
			table[nextHash&tableMask] = uint16(s)
			nextHash = hash(load32(source, nextS), shift)
			// if possible match equals a written literal, we have match
			if load32(source, s) == load32(source, candidate) {
				break
			}
		}

		// A 4-byte match has been found. We'll later see if more than 4 bytes
		// match. But, prior to the match, src[nextEmit:s] are unmatched. Emit
		// them as literal bytes.
		d += emitLiteral(destination[d:], source[nextEmit:s])

		// Call emitCopy, and then see if another emitCopy could be our next
		// move. Repeat until we find no match for the input immediately after
		// what was consumed by the last emitCopy call.
		//
		// If we exit this loop normally then we need to call emitLiteral next. We handle that
		// by proceeding to the next iteration of the main loop.
		for {
			// we have a 4 byte match at s from previous loop
			base := s

			// Extend the 4-byte match as long as possible.
			// In other words the largest k such that k <= len(src) and that
			// src[i:i+k-s] and src[s:k] have the same contents.
			s += 4
			for i := candidate + 4; s < len(source) && source[i] == source[s]; i, s = i+1, s+1 {
			}

			// Write the copy
			d += emitCopy(destination[d:], base-candidate, s-base)
			// update where we're at in source
			nextEmit = s

			// if s brings us over search limit, skip the rest of iteration
			if s >= sLimit {
				goto emitRemainder
			}

			// Next block is hyper-optimized code based on architecture and was
			// not necessary in implementation/in scope of our understanding, so
			// it is copied directly from implementation with original comments.

			// We could immediately start working at s now, but to improve
			// compression we first update the hash table at s-1 and at s. If
			// another emitCopy is not our next move, also calculate nextHash
			// at s+1. At least on GOARCH=amd64, these three hash calculations
			// are faster as one load64 call (with some shifts) instead of
			// three load32 calls.
			x := load64(source, s-1)
			prevHash := hash(uint32(x>>0), shift)
			table[prevHash&tableMask] = uint16(s - 1)
			currHash := hash(uint32(x>>8), shift)
			candidate = int(table[currHash&tableMask])
			table[currHash&tableMask] = uint16(s)
			if uint32(x>>8) != load32(source, candidate) {
				nextHash = hash(uint32(x>>16), shift)
				s++
				break
			}
		}
	}

	// label from lines 202 and 243 (if we would pass search limit)
emitRemainder:
	// Write literal if we have data left over from copy loops
	if nextEmit < len(source) {
		d += emitLiteral(destination[d:], source[nextEmit:])
	}
	// return the amount of Bytes written.
	return d
}

//--------------------------------------------------------------//
//---------------------decode functions-------------------------//

func decode(destination, source []byte) int {
	var d, s, offset, length int
	for s < len(source) {
		switch source[s] & 0x03 {
		case tagLiteral:
			//checks different literal casses
			x := uint32(source[s] >> 2)
			switch {
			case x < 60:
				s++
			case x == 60:
				s += 2
				// if there are to many bits to hold in one block
				//it passes it onto the next block.
				//checks for multiple overflow levels.
				if uint(s) > uint(len(source)) {
					return decodeErrCodeCorrupt
				}
				x = uint32(source[s-1])
			case x == 61: //case where x(the decimal representation) is 61 bytes
				s += 3
				if uint(s) > uint(len(source)) {
					return decodeErrCodeCorrupt
				}
				x = uint32(source[s-2]) | uint32(source[s-1])<<8
			case x == 62: // case with 62 bytes
				s += 4
				if uint(s) > uint(len(source)) {
					return decodeErrCodeCorrupt
				}
				x = uint32(source[s-3]) | uint32(source[s-2])<<8 | uint32(source[s-1])<<16
			case x == 63: // case with 63 bytes
				s += 5
				if uint(s) > uint(len(source)) {
					return decodeErrCodeCorrupt
				}
				x = uint32(source[s-4]) | uint32(source[s-3])<<8 | uint32(source[s-2])<<16 | uint32(source[s-1])<<24
			}
			//error check to make sure length is properly handled
			length = int(x) + 1
			if length <= 0 {
				return decodeErrCodeUnsupportedLiteralLength
			}
			if length > len(destination)-d || length > len(source)-s {
				return decodeErrCodeUnsupportedLiteralLength
			}
			copy(destination[d:], source[s:s+length])
			d += length
			s += length
			continue

			//impelements the same cases as above but for the tag copies
		case tagCopy1:
			s += 2
			if uint(s) > uint(len(source)) { // catches overflow from previous line
				return decodeErrCodeCorrupt
			}
			length = 4 + int(source[s-2])>>2&0x7
			offset = int(uint32(source[s-2])&0xe0<<3 | uint32(source[s-1]))

		case tagCopy2:
			s += 3
			if uint(s) > uint(len(source)) { // catches overflow from previous line
				return decodeErrCodeCorrupt
			}
			length = 1 + int(source[s-3])>>2
			offset = int(uint32(source[s-2]) | uint32(source[s-1])<<8)

		case tagCopy4:
			s += 5
			if uint(s) > uint(len(source)) { // catches overflow from previous line
				return decodeErrCodeCorrupt
			}
			length = 1 + int(source[s-5])>>2
			offset = int(uint32(source[s-4]) | uint32(source[s-3])<<8 | uint32(source[s-2])<<16 | uint32(source[s-1])<<24)
		}

		//copies sub-slice of the destination strinf into a new sub-slice
		//this runs directly through all strings even if overlap
		if offset >= length {
			copy(destination[d:d+length], destination[d-offset:])
			d += length
			continue
		}

		//makes checks to see if a and b lenght are the same
		//that way it can run without checks
		a := destination[d : d+length]
		b := destination[d-offset:]
		b = b[:len(a)]
		for i := range a {
			a[i] = b[i]
		}
		d += length
	}
	if d != len(destination) {
		return decodeErrCodeCorrupt
	}
	return 0
}

func decodedLen(source []byte) (blockLen, headerLen int, err error) {
	v, n := binary.Uvarint(source) // converitng it to binary
	if n <= 0 || v > 0xffffffff {  // checking if the length is incompatible
		return 0, 0, ErrCorrupt
	}
	const wordSize = 32 << (^uint(0) >> 32 & 1)
	if wordSize == 32 && v > 0x7fffffff { // check if length is too large
		return 0, 0, ErrTooLarge
	}
	return int(v), n, nil // returns length of decoded string
}

func Decode(destination, source []byte) ([]byte, error) {
	destLen, s, err := decodedLen(source)
	if err != nil {
		return nil, err
	}
	if destLen <= len(destination) { //if length is shorter
		//then we make destination write up to the destination length
		destination = destination[:destLen]
	} else {
		destination = make([]byte, destLen) // otherwise we write all of it
	}
	switch decode(destination, source[s:]) {
	case 0: // if decode returns case 0, return the destination string
		return destination, nil
	case decodeErrCodeUnsupportedLiteralLength:
		// else forget it and move on from error
		return nil, errUnsupportedLiteralLength
	}
	return nil, ErrCorrupt
}

//--------------------------------------------------------------//
//-------------------------main test----------------------------//

func main() {
	source := []byte("Wikipedia is a free, web-based, collaborative, multilingual encyclopedia project.")
	encoded := Encode(nil, source)
	fmt.Println("encoding")
	fmt.Println(encoded)
	fmt.Println("decoding")
	decoded, err := Decode(nil, encoded)
	decodedString := string(decoded[:])
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(decodedString)
}
