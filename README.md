# Snappy Compression Algorithm Golang Implementation
### Cole Marco, Satchel Sevenau

## What is Snappy?
Previously known as Zippy, it's a lossless, data compression algorithm implemented by Google used primarily on text. The algorithm focuses on the time-efficient compression of the data rather than peak compression ratio. This means it has a mediocre compression rate of 1.5x to 1.7x for plain text and 2x-4x for HTML, but can compress and decompress at rates much faster than other algorithms. Snappy uses 64-bit operations to allow efficient operation on multiple cores of modern consumer processors, boasting speeds upward of 250 MB/s compress and 500 MB/s decompress on just a single core of a 2015 i7 CPU. Part of this speed differential comes from the fact that Snappy doesn't use an entropy coder, like a Huffman tree or arithmetic encoder.

## Applications for Snappy
We're used to compression algorithms focusing almost entirely on compression ratio, but it turns out there's some intuitive applications for a high data-rate algorithm. Google has used it for multiple of its own projects, such as BigTable, MapReduce, MongoDB, and its own RPC system. A keen reader may recognize many of these as database tools, which makes total sense when considering what is going on under the hood. A database receives a query, and then returns all matching results. In a small business, this may be 100s of lines of text, but on the scale of a tech giant, these results could easily be 100,000 lines of text. So since compute resources are generally more expensive than storage resources, Google can use Snappy to make these queries ultra fast, at the (lower) cost of storage.

## High-Level Algorithm Explanation

Snappy operates with both a block-level and stream format. We will focus on the stream format, since the data is only chunked between blocks for large files, which doesn't provide an accessible introduction to the algorithm. Let's start off with a string that we'd like to encode:

`"Wikipedia is a free, web-based, collaborative, multilingual encyclopedia project."` 

Snappy will turn this string into an array of bytes because it is byte-oriented, rather than bit oriented. We first start off by writing the length of the uncompressed data (as a little-endian varint) at the beginning of our compressed stream. Once the compression has started, we have four different element types to consider:

1. Literal or uncompressed data. Stored with a 00 tag-byte, the upper 6 bis are used to store the length of the literal. Lengths larger than 60 can be encoded in multiple bytes, with 60 representing 1 byte, 61 representing 2, 62 is 3 bytes, and 63 is 4.
2. A copy with tagbyte 01. Length is stored as 3 bits and offset stored as 11; one byte after tag byte is used for part of the offset.
3. A copy with tagbyte 10. Length is stored as 6 bits in the tag byte and offset stored as 2 byte integer after tag byte.
4. A copy with tagbyte 11. Length is stored as 6 bits in the tag byte and offset stored as 4 byte little-endian integer after the tag byte.

A literal, as mentioned before is just the uncompressed data fed directly as a byte array into the compressed stream. A copy on the other hand is essentially an indicator, that whatever is encoded there has been encoded previously in the stream. For our example, both "Wikipedia" and "encyclopedia" have the term "pedia". A copy at encyclopedia would store an offset that would tell the algorithm how many bytes to look back in the stream, and then use whatever is at that location instead of storing it literally. 

We'll run through the example with actual data now. When we encode our stream and represent it as hex, we get the following.

```
ca02 f042 5769 6b69 7065 6469 6120 6973
2061 2066 7265 652c 2077 6562 2d62 6173
6564 2c20 636f 6c6c 6162 6f72 6174 6976
652c 206d 756c 7469 6c69 6e67 7561 6c20
656e 6379 636c 6f09 3ff0 1470 726f 6a65
6374 2e00 0000 0000 0000 0000 0000 0000
```

But we'll focus on the first line.

`ca02 f042 5769 6b69 7065 6469 6120 6973  ...BWikipedia is`

On the left hand side we have the compressed stream in hex, and the right shows us whay it decodes to. If the bytes are a part of a format element and not actually part of the text, we'll represent it with a period.

Remember we said the stream starts with the length of the uncompressed data stored as a little-endian varint. In our case, the bytes `0xca02`. When decoded into binary we get the following:

`11001010 00000010`

The little-endian varint specification for Snappy says that the lower seven bits of each byte in stream header is used for the length data, and the high bit is a flag to indicate the end of the length field. Since our data is little-endian, we need to flip the hex bits like so:

`00000010 11001010`

The first byte has a 0 in the MSB, so we know there is at least one more byte following this which stores the length of the uncompressed data. The next byte has a 1 in the MSB, so we know that is the final length byte. concatenating the lower seven bytes from each one gives:

`00000101001010`

Or 330 in decimal, which is exactly the length in bytes of the uncompressed data.

Moving on to our next element in the stream, `0xf042`. Converting to binary gives us:

`11110000 01000010`

Since the element type is encoded in the lower two bits of the tag byte, we know the element here is `00` or a literal. Recall the upper 6 bits of a literal represent the length if it is under 60, or how many proceeding bytes will store the length if it's between 60 and 63. In our case, `111100` is 60 in decimal, which means 1 proceeding byte stores the length of the literal. Decoding that following byte gives us 66 in decimal (note that the snappy format encodes the length of the literal as length-1), so our final literal length is 67. That means the following 67 bytes are not compressed and can be read litearlly as text. Sure enough:

ca02 f042 **5769 6b69 7065 6469 6120 6973**  
**2061 2066 7265 652c 2077 6562 2d62 6173**  
**6564 2c20 636f 6c6c 6162 6f72 6174 6976**  
**652c 206d 756c 7469 6c69 6e67 7561 6c20**  
**656e 6379 636c 6f**09 3ff0 1470 726f 6a65  
6374 2e00 0000 0000 0000 0000 0000 0000

or the next 67 bits can be directly translated from hex to ASCII, giving the following output:

`Wikipedia is a free, web-based, collaborative, multilingual encyclo`

The byte directly afterwards is `0x09`, or in binary:

`00001001`

Thus, this element type is `01` or a copy where the length is stored as 3 bits and the offset is stored as the remaining 3 bits in the tagbyte, and 8 bits in the following byte. Thus, our full copy element is:

`00001001 00111111`

The length is encoded as 2 and the offset is 63. If we move back 63 bytes, we reach the part in the literal corresponding to the "pedia " in "Wikipedia ". Thus, this copy corresponds to "pedia ". This makes sense, as we are currently at the "pedia " in "encyclopedia " in the input stream. 

Finally, `0xf014` represents a literal with length of 21 bytes. This gives us the completed encoding of 

```
ca02 f042 5769 6b69 7065 6469 6120 6973  ...BWikipedia is
2061 2066 7265 652c 2077 6562 2d62 6173   a free, web-bas
6564 2c20 636f 6c6c 6162 6f72 6174 6976  ed, collaborativ
652c 206d 756c 7469 6c69 6e67 7561 6c20  e, multilingual
656e 6379 636c 6f09 3ff0 1470 726f 6a65  encyclo.?..proje
6374 2e00 0000 0000 0000 0000 0000 0000  ct.
```

## Golang implemetation encode walkthough

The public `Encode` function is the high-level handler for the encoding process (pasted below for convenience).

``` go
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
```

### Encode
`Encode` is the primary function that handles the overarching concepts in the encoding process. It relies on multiple helper that functions that we'll detail below. For more information on any of these processes, the code is commented for better documentation. `Encode` first checks for and allocates the appropriate arrays for the destination stream and overflow. It also keeps track of the number of bytes written from the source stream, so it knows where to update the stream pointer in the destination stream. `Encode` then starts searching for and encoding copies/literals in the destination stream until the entire source has been compressed and written to the destination stream. It then returns the encoded byte array. Let's go over the helper functions in order of when they appear in `Encode`.

### MaxEncodedLen
`MaxEncodedLen` simply returns the maximum number of bytes the encoded stream could take up. We can efficiently calculate our worst case compression rate without running through the full encoding process, allowing us to guard against the destination array being too small to contain the encoded output. Here's an analysis of the space blowup:

The trailing literal sequence has a space blowup of at most 62/60 since a literal of length 60 needs one tag byte + one extra byte for length information. Item blowup is trickier to measure. Suppose the "copy" op copies 4 bytes of data. Because of a special check in the encoding code, we produce a 4-byte copy  only if the offset is < 65536. Therefore the copy op takes 3 bytes to encode, and this type of item leads to at most the 62/60 blowup for representing literals.

Suppose the "copy" op copies 5 bytes of data. If the offset is big enough, it will take 5 bytes to encode the copy op. Therefore the worst case here is a one-byte literal followed by a five-byte copy. That is, 6 bytes of input turn into 7 bytes of "compressed" data. This last factor dominates the blowup, so the final estimate is `32 + len(source) + len(source)/6`.

`MaxEncodedLen` then compares this max space blowup value and sees if it's larger than `0xffffffff`. If it is, it returns as error because the max blowup would potentially be larger than the destination capacity. If it's not, it returns the max number of bytes source could be encoded to.

### emitLiteral
`emitLiteral` is called in `Encode` when the amount of bytes left to write is too small for a copy to be worth making. In other words, if we were to try to compress this, it always results in a space blowup > 1, thus we write a literal to the stream instead. The function works by passing in the byte array we are trying to write as a literal and a destination array. `emitLiteral` then follows the rules we set up for literals in the high-level Snappy explanation above. If the length is 60 or greater, it uses the appropriate amount of bytes to store the length and concatenates the tag literal of `00` to the end of the tag byte. Finally, it writes the literal after tag and length bytes.

### encodeBlock
`encodeBlock` handles the main (stream level not block format) compression/encoding of the snappy algorithm (finding matches and turning them into copies). It first initializes a hash table (or dictionary) in which we'll store the literal bytes we've written and their location in the stream. We only hash a new value (add an item to the dictionary) when a new literal is written. 

The encoder uses a method Google calls "Heuristic match skipping": If 32 bytes are scanned with no matches found, start looking only at every other byte. If 32 more bytes are scanned (or skipped), look at every third byte, etc.. When a match is found, immediately go back to looking at every byte. This has a small performace decrease, but for non-compressible data (such as JPEG) it's a huge win since the compressor quickly "realizes" the data is incompressible and doesn't bother looking for matches everywhere!

To implement this, the encoder begins by defining the skip variable that keeps track of how many bytes there have been since the last copy match. It then checks at the stream pointer if it can find any matches in the hash table according to the method described above. Once a 4-byte match has been found, we emit any bytes previous to the match as literals (since they had no match). Next we write the copy, and see if the next bytes immediately after could be a copy as well. Repeat until we can't write a copy.

We follow this process until all of the source input has been encoded to the destination stream. If at any point of this procedure, we get too close to the end of the source stream, we immediately write the rest of the stream as a literal. We then return the amount of bytes written such that `Encode` can keep track of the source, and destination stream pointers (in case `Encode` only passed in 1 block because the file was large enough to need to be chunked).

## Golang implementation decode walkthrough

### Decode
The public `Decode` function is the high-level handler for the decoding process (pasted below for convenience). 

``` go
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
```

The `Decode` function is the primary function to handle the decoding process. It takes in the arrays destination and source. It checks if the length of the destination is too big and saves only up to the amount of allocated space. The rest is saved and handled later. It passes this new array onto the ‘decode’ function, which actually decompresses the array that is passed to it.

### decode
The `decode` function handles all of the possible cases and utilizes the tag literals to decompress the string. There are several cases when the length of the code is less than 60, equal to 60, equal to 61, equal to 62, and equal to 63. The ‘decode’ function handles all of these and this is where the tag literal is important because it informs the algorithm how it should decompress the data. It decompresses it into blocks of information, but the tag literals inform how much more data will come in the following blocks so that it can put it into the proper stream format. It then also checks if the tags are copies, so it can find the section of the array it was copied from.

### decodeLen
The `decodeLen` function is a helper function to process the length of the decoded array will be. It converts the source array to binary and returns the length of the block and the number of bytes that have been decoded. We can use this number to keep track of where we are in the stream.
