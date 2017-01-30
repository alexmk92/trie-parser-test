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
	line = "Sneeki auctions, 'BUYING Rogue Epic : D  // SELLING Shiny Brass Idol 700pp PST'"
	//line = "Stockmarket auctions, 'WTS Mystic/Imbued Koada`Dal Mithril & Dwarven Cultural Armor Full sets and Individual Pieces available'"
	//line = "'WTS Mithril Greaves 750 , Lodizal Shell Boots 1.5 , Runed Lava Pendant 800, Fire Emerald Platinum Ring x2 750pp, Orc Fang Earring x2 400pp'"
	line = "[Mon Jan 09 20:34:30 2017] Kandaar auctions, 'WTS Mithril Greaves 750 , Lodizal Shell Boots 1.5 , Runed Lava Pendant 800, Fire Emerald Platinum Ring x2 750pp, Orc Fang Earring x2 400pp'"

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
		// extract the item name
		//fmt.Println("Checking if: " + string(buffer) + " is a prefix")
		if trie.HasPrefix(strings.TrimSpace(string(buffer))) {
			prevMatch = string(buffer)
			fmt.Println("Has prefix: ", string(buffer))
			if i == len(line)-1 {
				buffer = []byte{}
				appendIfInTrie(prevMatch, selling, &a.Items)
				prevMatch = ""
				skippedChar = []byte{}
			}
		} else if prevMatch != "" {
			fmt.Println("Prev was: ", prevMatch)
			skippedChar = append(skippedChar, byte(c))
			buffer = []byte{}
			appendIfInTrie(prevMatch, selling, &a.Items)
			prevMatch = ""
		} else if !parsePriceAndQuantity(&buffer, &a) {
			fmt.Println(prevMatch)
			prevMatch = ""
			buffer = []byte{}
			skippedChar = []byte{}
		}
		// Just continue execution, nothing else to be caught here!
	}
	fmt.Println("Buffer is: ", string(buffer))
	fmt.Println("Is sell mode? ", selling)
	fmt.Println("Items is: ", &a.Items)
	fmt.Println("Total items: ", fmt.Sprint(len(a.Items)))
}

func parsePriceAndQuantity(buffer *[]byte, auction *Auction) bool {
	fmt.Println("Parsing: " + string(*buffer) + " for price data")
	price_regex := regexp.MustCompile(`(?im)^(x ?)?(\d*\.?\d*)(k|p|pp| ?x)?$`)
	price_string := strings.TrimSpace(string(*buffer))

	matches := price_regex.FindStringSubmatch(price_string)
	if len(matches) > 1 && len(strings.TrimSpace(matches[0])) > 0 && len(auction.Items) > 0 {
		matches = matches[1:]
		price, err := strconv.ParseFloat(strings.TrimSpace(matches[1]), 64)
		if err != nil {
			//fmt.Println("error setting price: ", err)
			price = 0.0
		}
		var delimiter string = strings.ToLower(matches[2])
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

		//fmt.Println("Delimeter is: ", delimiter)

		// check if this was in-fact quantity data
		var item *Item = &auction.Items[len(auction.Items)-1]
		if isQuantity == true && price > 0.0 {
			//fmt.Println("setting quantity: ", fmt.Sprint(int16(price)))
			item.Quantity = int16(price)
		} else if price > 0.0 {
			// if we had WTS GEBS 1.5 we would assume 1.5 = 1.5k = 1500
			price_without_delim_regex := regexp.MustCompile(`(?im)^([0-9]{1,}\.[0-9]{1,})$`)
			matches = price_without_delim_regex.FindAllString(price_string, -1)
			if len(matches) > 0 {
				fmt.Println("Parsed: " + price_string + " got: ", matches)
				multiplier = 1000.0
			}

			item.Price = float32(price * multiplier)
		}

		return true
	}

	return false
}