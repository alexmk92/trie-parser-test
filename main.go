package main

import (
	"fmt"
	"github.com/fvbock/trie"
	"strings"
	"regexp"
	"errors"
	"github.com/alexmk92/stringutil"
	"strconv"
)

// Global connection to be used by the server
var DB = Database{}

type Auction struct {
	Seller string
	Timestamp string
	Items []Item
	itemLine string
	raw string
}

type Item struct {
	Name string
	Price float32
	Quantity int16
	ForSale bool
}

func extractParserInformationFromLine(line string, auction *Auction) error {
	fmt.Println("Attempting to match: ", line)
	reg := regexp.MustCompile(`(?m)^\[(?P<Timestamp>[A-Za-z0-9: ]+)+] (?P<Seller>[A-Za-z]+) auction[s]?, '(?P<Items>.+)'$`)
	matches := reg.FindStringSubmatch(line)
	if len(matches) == 0 {
		return errors.New("No matches found for expression")
	}

	auction.raw = line
	auction.Timestamp = matches[0]
	auction.Seller = matches[1]
	auction.itemLine = matches[2]

	return nil
}

func main() {
	// Initialise DB connections
	fmt.Println("Initialising database connection")
	DB.Open()
	fmt.Println("Connection initialised")

	// Create our trie
	trie := trie.NewTrie()

	// Add trade grammar to the trie so we dont empty buffer on sales
	trie.Add("selling")
	trie.Add("buying")
	trie.Add("wtb")
	trie.Add("wts")

	// Load data into our trie
	fmt.Println("Populating table")
	itemQuery := "SELECT displayName, id FROM items ORDER BY displayName ASC"
	rows, _ := DB.Query(itemQuery)
	if rows != nil {
		for rows.Next() {
			var name string
			var id int64

			rows.Scan(&name, &id)
			if id > 0 && name != "" {
				trie.Add(strings.ToLower(name))
			}
		}
	}

	fmt.Println("Populated")

	appendIfInTrie := func(key string, forSale bool, out *[]Item) {
		//fmt.Println("Checking if: " + key + " can be output")
		if trie.Has(strings.TrimSpace(key)) {
			//fmt.Println("JUP")
			*out = append(*out, Item {
				Name: strings.TrimSpace(key),
				Price: 0.0,
				Quantity: 1.0,
				ForSale: forSale,
			})
		}
	}

	// Try to find item
	line := "wts 10 dose blood of the wolf wurmslayermask of wurms>|$swiftwindearthcallerwurmslayerdagger of marnek 250k today I've got some other cool items, WTB mask of wurmsAle 500p spear of fate 50k x2 bronze girdle"
	//line = "Sneeki auctions, 'BUYING Rogue Epic : D  // SELLING Shiny Brass Idol 700pp PST'"
	//line = "Stockmarket auctions, 'WTS Mystic/Imbued Koada`Dal Mithril & Dwarven Cultural Armor Full sets and Individual Pieces available'"
	//line = "'WTS Mithril Greaves 750 , Lodizal Shell Boots 1.5 , Runed Lava Pendant 800, Fire Emerald Platinum Ring x2 750pp, Orc Fang Earring x2 400pp'"
	//line = "[Mon Jan 09 20:34:30 2017] Kandaar auctions, 'WTS Mithril Greaves 750 , Lodizal Shell Boots 1.5 , Runed Lava Pendant 800, Fire Emerald Platinum Ring x2 750pp, Orc Fang Earring x2 400pp'"
	//line = "[Mon Feb 15 17:49:20 2016] Joeleen auctions, 'WTS Cat Eye Platinum Necklace 150p | Rune Etched Wedding Band 400p | Hexed Kerran Doll 200p'"
	//line = "[Mon Feb 15 23:25:46 2016] Babanker auctions, 'WTS Chetari Wardstaff 3k, Crushed Topaz 400pp, Crushed Lava Ruby 300pp, Gauntlets of Iron Tactics 1.5k, Circlet of Shadow 3k, Cold Steel Vambraces 500pp, '"

	a := Auction{}
	err := extractParserInformationFromLine(line, &a)
	if err != nil {
		fmt.Println(err.Error())
	} else {
		fmt.Println(a.Seller)
		fmt.Println(a.Timestamp)
		fmt.Println(a.raw)
		fmt.Println(a.itemLine)
	}

	buffer := []byte{}
	selling := true
	skippedChar := []byte{} // use an array so we can check the size

	a.itemLine = line

	// NOTE: We use Go's `continue` kewyword to break execution flow instead of
	// chaining else-if's.  I personally find this more readable with the
	// comment blocks above each part of the parser!!
	var prevMatch string = ""
	for i, c := range strings.ToLower(a.itemLine) {
		buffer = append(buffer, byte(c))

		// check for selling
		if stringutil.CaseInsenstiveContains(string(buffer), "wts", "selling") {
			selling = true
			buffer = []byte{}
			prevMatch = ""
			skippedChar = []byte{}
			continue
		}

		// check for buying
		if stringutil.CaseInsenstiveContains(string(buffer), "wtb", "buying", "trading") {
			selling = false
			buffer = []byte{}
			prevMatch = ""
			skippedChar = []byte{}
			continue
		}

		// check if we skipped a letter on the previous iteration and shift the items forward
		// this checks example: wurmslayerale it would fail at wurmslayera we set "a" as the
		// skipped character, extract wurmslayer and the begin to match ale using the "a" char
		// once we append to the buffer we reset the skipped char to avoid prepending on
		// subsequent calls
		if len(skippedChar) > 0 {
			buffer = append(skippedChar, buffer...)
			skippedChar = []byte{}
		}

		// check if the current string exists in the buffer, we trim any spaces
		// from the left but not the right as that can skew the results
		// if we find a match store the previous match, for the next iteration.
		// finally we check to see if we're at the last position in the line,
		// if we are then we reset the buffer and attempt to append to the trie
		// if our buffer contains a match
		if trie.HasPrefix(strings.TrimLeft(string(buffer), " ")) {
			prevMatch = string(buffer)
			fmt.Println("Has prefix: ", string(buffer))
			if i == len(line)-1 {
				buffer = []byte{}
				appendIfInTrie(prevMatch, selling, &a.Items)
				prevMatch = ""
				skippedChar = []byte{}
			}

			continue
		}
		// The trie did not have the prefix composed of the char buffer, we now evaluate
		// the "previousMatch" which is the buffer string n-1.  We can assume that
		// on this iteration the new character accessed caused the buffer to be
		// invalidated on the item trie, therefore we append this character
		// into the skippedChar byte and then clear the buffer.
		// On our next iteration we populate the buffer with this "skipped"
		// character in order to build the next item line...
		// this allows us to catch cases where items are budged
		// up against one another without separators such as
		// wurmslayerswiftwindale would allow us to extract:
		// wurmslayer swiftwind ale
		// NOTE: We don't reset the buffer in this method as we always want
		// to check for Pricing and Quantity data, we will only reset
		// the buffer if no match is found for meta information about
		// the current item!
		if prevMatch != "" {
			fmt.Println("Prev was: ", prevMatch)

			// We don't want to put spaces back into the buffer, the whole purpose of
			// skippedChar is to catch cases where uses budge items together.
			// Therefore we will only append non space characters.
			if string(byte(c)) != " " { skippedChar = append(skippedChar, byte(c)) }

			appendIfInTrie(prevMatch, selling, &a.Items)
		}
		// This is the final part of the parser, the previous block will have added a
		// new item to the auction items array if it found a match in the trie, otherwise
		// the array will remain the same.
		// At this point we want to extract any meta information for this item.  We can
		// assume that the buffer now contains information like " x2 50p" which we want
		// to extract and assign to the item.   If however none of our price extraction
		// reg-exs find a match we set "prevMatch" back to null and we also empty our buffer
		// as we have now essentially exhausted our search for this item
		if !parsePriceAndQuantity(&buffer, &a) {
			prevMatch = ""
			buffer = []byte{}

			continue
		}
		// Just continue execution, nothing else to be caught here - this means that we have
		// successfully extracted meta information, woot!
	}
	fmt.Println("Buffer is: ", string(buffer))
	fmt.Println("Is sell mode? ", selling)
	fmt.Println("Items is: ", &a.Items)
	fmt.Println("Total items: ", fmt.Sprint(len(a.Items)))
}

// This method should be fairly self explanatory.  We simply use a regex to
// extract matches from the input string and then write the data back out
// to the last item on the input struct (this assumes that meta info is in
// the order of ITEM QUANTITY PRICE or ITEM PRICE QUANTITY etc.
// if the order is QUANTITY ITEM PRICE then the quantity will be assigned to
// the prior item (assuming auction.items > 1) and the price would be assigned
// to the correct item, if however auction.items == 0 then the extracted meta
// inf is lost, this could possibly be parsed correctly by storing a buffer
// of prices for previously parsed items when the legnth is 0, as we could then
// assume that the rest of the items would follow the same pattern in that string...(TODO?)
func parsePriceAndQuantity(buffer *[]byte, auction *Auction) bool {
	fmt.Println("Parsing: " + string(*buffer) + " for price data")
	price_regex := regexp.MustCompile(`(?im)^(\d*\.?\d*)(k|p|pp)?$`)
	price_string := string(*buffer)

	// I don't think we need this as we now prevent spaces from being set as "SkipChar"
	//if price_string[len(price_string)-1:] == " " {
	//	fmt.Println("ITS A SPACE")
	//	return false
	//}
	price_string = strings.TrimSpace(price_string)

	matches := price_regex.FindStringSubmatch(price_string)
	if len(matches) > 1 && len(strings.TrimSpace(matches[0])) > 0 && len(auction.Items) > 0 {
		matches = matches[1:]
		price, err := strconv.ParseFloat(strings.TrimSpace(matches[0]), 64)
		if err != nil {
			fmt.Println("error setting price: ", err)
			price = 0.0
		}
		var delimiter string = strings.ToLower(matches[1])
		var multiplier float64 = 1.0;
		var isQuantity bool = false

		switch delimiter {
			case "x": isQuantity = true; break;
			case "p": multiplier = 1.0; break;
			case "k": multiplier = 1000.0; break;
			case "pp": multiplier = 1.0; break;
			case "m": multiplier = 1000000.0; break;
			default: multiplier = 1; break;
		}

		fmt.Println("Delimeter is: ", delimiter)

		// check if we need to set a new multiplier
		price_without_delim_regex := regexp.MustCompile(`(?im)^([0-9]{1,}\.[0-9]{1,})$`)
		matches = price_without_delim_regex.FindAllString(price_string, -1)
		if len(matches) > 0 {
			fmt.Println("Parsed: " + price_string + " got: ", matches)
			multiplier = 1000.0
		}

		// check if this was in-fact quantity data
		var item *Item = &auction.Items[len(auction.Items)-1]
		if isQuantity == true && price > 0.0 {
			//fmt.Println("setting quantity: ", fmt.Sprint(int16(price)))
			item.Quantity = int16(price)
		} else if price > 0.0 && float32(price * multiplier) > item.Price {
			item.Price = float32(price * multiplier)
		}

		return true
	}

	return false
}