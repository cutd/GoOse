/*
This is a golang port of "Goose" originaly licensed to Gravity.com
under one or more contributor license agreements.  See the NOTICE file
distributed with this work for additional information
regarding copyright ownership.

Golang port was written by Antonio Linari

Gravity.com licenses this file
to you under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance
with the License.  You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package goose

import (
	"code.google.com/p/go.net/html"
	"code.google.com/p/go.net/html/atom"
	"github.com/PuerkitoBio/goquery"
	"github.com/fatih/set"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const DEFAULT_LANGUAGE = "en"

var MOTLEY_REPLACEMENT = "&#65533;"
var ESCAPED_FRAGMENT_REPLACEMENT = regexp.MustCompile("#!")
var TITLE_REPLACEMENTS = regexp.MustCompile("&raquo;")
var PIPE_SPLITTER = regexp.MustCompile("\\|")
var DASH_SPLITTER = regexp.MustCompile(" - ")
var ARROWS_SPLITTER = regexp.MustCompile("»")
var COLON_SPLITTER = regexp.MustCompile(":")
var SPACE_SPLITTER = regexp.MustCompile(" ")
var A_REL_TAG_SELECTOR = "a[rel=tag]"
var A_HREF_TAG_SELECTOR = [...]string{"/tag/", "/tags/", "/topic/", "?keyword"}
var RE_LANG = "^[A-Za-z]{2}$"

type contentExtractor struct {
	config configuration
}

func NewExtractor(config configuration) contentExtractor {
	return contentExtractor{
		config: config,
	}
}

func (this *contentExtractor) getTitle(article *Article) string {
	title := ""
	doc := article.Doc

	titleElement := doc.Find("title")
	if titleElement == nil || titleElement.Size() == 0 {
		return title
	}

	titleText := titleElement.Text()
	usedDelimiter := false

	if strings.Contains(titleText, "|") {
		titleText = this.splitTitle(RegSplit(titleText, PIPE_SPLITTER))
		usedDelimiter = true
	}

	if !usedDelimiter && strings.Contains(titleText, "-") {
		titleText = this.splitTitle(RegSplit(titleText, DASH_SPLITTER))
		usedDelimiter = true
	}

	if !usedDelimiter && strings.Contains(titleText, "»") {
		titleText = this.splitTitle(RegSplit(titleText, ARROWS_SPLITTER))
		usedDelimiter = true
	}

	if !usedDelimiter && strings.Contains(titleText, ":") {
		titleText = this.splitTitle(RegSplit(titleText, COLON_SPLITTER))
		usedDelimiter = true
	}

	title = strings.Replace(titleText, MOTLEY_REPLACEMENT, "", -1)
	return title
}

func (this *contentExtractor) splitTitle(titles []string) string {
	largeTextLength := 0
	largeTextIndex := 0
	for i, current := range titles {
		if len(current) > largeTextLength {
			largeTextLength = len(current)
			largeTextIndex = i
		}
	}
	title := titles[largeTextIndex]
	title = strings.Replace(title, "&raquo;", "»", -1)
	return title
}

func (this *contentExtractor) getMetaLanguage(article *Article) string {
	language := ""
	doc := article.Doc

	attr, _ := doc.Attr("lang")
	if attr == "" {
		selection := doc.Find("meta").EachWithBreak(func(i int, s *goquery.Selection) bool {
			attr, exists := s.Attr("http-equiv")
			if exists && attr == "content-language" {
				return false
			}
			return true
		})
		if selection != nil {
			attr, _ = selection.Attr("content")
		}
	}
	language = attr
	if language == "" {
		language = DEFAULT_LANGUAGE
	}
	return language
}

func (this *contentExtractor) getFavicon(article *Article) string {
	favicon := ""
	doc := article.Doc
	doc.Find("link").EachWithBreak(func(i int, s *goquery.Selection) bool {
		attr, exists := s.Attr("rel")
		if exists && strings.Contains(attr, "icon") {
			favicon, _ = s.Attr("href")
			return false
		}
		return true
	})
	return favicon
}

func (this *contentExtractor) getMetaContentWithSelector(article *Article, selector string) string {
	content := ""
	doc := article.Doc
	selection := doc.Find(selector)
	content, _ = selection.Attr("content")
	return content
}

func (this *contentExtractor) getMetaContent(article *Article, metaName string) string {
	content := ""
	doc := article.Doc
	doc.Find("meta").EachWithBreak(func(i int, s *goquery.Selection) bool {
		attr, exists := s.Attr("name")
		if exists && attr == metaName {
			content, _ = s.Attr("content")
			return false
		}
		return true
	})
	return content
}

func (this *contentExtractor) getMetaContents(article *Article, metaNames *set.Set) map[string]string {
	contents := make(map[string]string)
	doc := article.Doc
	counter := metaNames.Size()
	doc.Find("meta").EachWithBreak(func(i int, s *goquery.Selection) bool {
		attr, exists := s.Attr("name")
		if exists && metaNames.Has(attr) {
			content, _ := s.Attr("content")
			contents[attr] = content
			counter--
			if counter < 0 {
				return false
			}
		}
		return true
	})
	return contents
}

func (this *contentExtractor) getMetaDescription(article *Article) string {
	return this.getMetaContent(article, "description")
}

func (this *contentExtractor) getMetKeywords(article *Article) string {
	return this.getMetaContent(article, "keywords")
}

func (this *contentExtractor) getDomain(article *Article) string {
	finalUrl := article.FinalUrl
	u, err := url.Parse(finalUrl)
	if err == nil {
		return u.Host
	}
	return ""
}

func (this *contentExtractor) getTags(article *Article) *set.Set {
	tags := set.New()
	doc := article.Doc
	selections := doc.Find(A_REL_TAG_SELECTOR)
	selections.Each(func(i int, s *goquery.Selection) {
		tags.Add(s.Text())
	})
	selections = doc.Find("a")
	selections.Each(func(i int, s *goquery.Selection) {
		href, exists := s.Attr("href")
		if exists {
			for _, part := range A_HREF_TAG_SELECTOR {
				if strings.Contains(href, part) {
					tags.Add(s.Text())
				}
			}
		}
	})

	return tags
}

func (this *contentExtractor) calculateBestNode(article *Article) *goquery.Selection {
	doc := article.Doc
	var topNode *goquery.Selection
	nodesToCheck := this.nodesToCheck(doc)

	startingBoost := 1.0
	cnt := 0
	i := 0
	parentNodes := set.New()
	nodesWithText := make([]*goquery.Selection, 0)
	for _, node := range nodesToCheck {
		textNode := node.Text()
		ws := this.config.stopWords.stopWordsCount(this.config.targetLanguage, textNode)
		highLinkDensity := this.isHighLinkDensity(node)
		if ws.stopWordCount > 2 && !highLinkDensity {
			nodesWithText = append(nodesWithText, node)
		}
	}
	nodesNumber := len(nodesWithText)
	negativeScoring := 0
	bottomNegativeScoring := float64(nodesNumber) * 0.25

	for _, node := range nodesWithText {
		boostScore := 0.0
		if this.isBoostable(node) {
			if cnt >= 0 {
				boostScore = float64((1.0 / startingBoost) * 50)
				startingBoost += 1
			}
		}

		if nodesNumber > 15 {
			if float64(nodesNumber-i) <= bottomNegativeScoring {
				booster := bottomNegativeScoring - float64(nodesNumber-i)
				boostScore = -math.Pow(booster, 2.0)
				negScore := math.Abs(boostScore) + float64(negativeScoring)
				if negScore > 40 {
					boostScore = 5.0
				}
			}
		}

		textNode := node.Text()
		ws := this.config.stopWords.stopWordsCount(this.config.targetLanguage, textNode)
		upScore := ws.stopWordCount + int(boostScore)
		parentNode := node.Parent()
		this.updateScore(parentNode, upScore)
		this.updateNodeCount(parentNode, 1)
		if !parentNodes.Has(parentNode) {
			parentNodes.Add(parentNode)
		}
		parentParentNode := parentNode.Parent()
		if parentParentNode != nil {
			this.updateNodeCount(parentParentNode, 1)
			this.updateScore(parentParentNode, upScore/2.0)
			if !parentNodes.Has(parentParentNode) {
				parentNodes.Add(parentParentNode)
			}
		}
		cnt++
		i++
	}

	topNodeScore := 0
	parentNodesArray := parentNodes.List()
	for _, p := range parentNodesArray {
		e := p.(*goquery.Selection)
		score := this.getScore(e)
		if score >= topNodeScore {
			topNode = e
			topNodeScore = score
		}
		if topNode == nil {
			topNode = e
		}
	}
	return topNode
}

func (this *contentExtractor) getScore(node *goquery.Selection) int {
	return this.getNodeGravityScore(node)
}

func (this *contentExtractor) getNodeGravityScore(node *goquery.Selection) int {
	grvScoreString, exists := node.Attr("gravityScore")
	if !exists {
		return 0
	}
	grvScore, err := strconv.Atoi(grvScoreString)
	if err != nil {
		return 0
	}
	return grvScore
}

func (this *contentExtractor) updateScore(node *goquery.Selection, addToScore int) {
	currentScore := 0
	scoreString, _ := node.Attr("gravityScore")
	if scoreString != "" {
		currentScore, _ = strconv.Atoi(scoreString)
	}
	newScore := currentScore + addToScore
	this.setAttr(node, "gravityScore", strconv.Itoa(newScore))
}

func (this *contentExtractor) updateNodeCount(node *goquery.Selection, addToCount int) {
	currentScore := 0
	scoreString, _ := node.Attr("gravityNodes")
	if scoreString != "" {
		currentScore, _ = strconv.Atoi(scoreString)
	}
	newScore := currentScore + addToCount
	this.setAttr(node, "gravityNodes", strconv.Itoa(newScore))
}

func (this *contentExtractor) isBoostable(node *goquery.Selection) bool {
	flag := false
	para := "p"
	stepsAway := 0
	minimumStopwordCount := 5
	maxStepsawayFromNode := 3

	nodes := node.Siblings()
	nodes.EachWithBreak(func(i int, s *goquery.Selection) bool {
		currentNodeTag := s.Get(0).DataAtom.String()
		if currentNodeTag == para {
			if stepsAway >= maxStepsawayFromNode {
				flag = false
				return false
			}
			paraText := s.Text()
			ws := this.config.stopWords.stopWordsCount(this.config.targetLanguage, paraText)
			if ws.stopWordCount > minimumStopwordCount {
				flag = true
				return false
			}
			stepsAway++
			return true
		}
		return true
	})

	return flag
}

func (this *contentExtractor) nodesToCheck(doc *goquery.Document) []*goquery.Selection {
	output := make([]*goquery.Selection, 0)
	tags := []string{"p", "pre", "td"}
	for _, tag := range tags {
		selections := doc.Find(tag)
		if selections != nil {
			selections.Each(func(i int, s *goquery.Selection) {
				output = append(output, s)
			})
		}
	}
	return output
}

func (this *contentExtractor) isHighLinkDensity(node *goquery.Selection) bool {
	links := node.Find("a")
	if links == nil || links.Size() == 0 {
		return false
	}
	text := node.Text()
	words := strings.Split(text, " ")
	nwords := len(words)
	sb := make([]string, 0)
	links.Each(func(i int, s *goquery.Selection) {
		linkText := s.Text()
		sb = append(sb, linkText)
	})
	linkText := strings.Join(sb, "")
	linkWords := strings.Split(linkText, " ")
	nlinkWords := len(linkWords)
	nlinks := links.Size()
	linkDivisor := float64(nlinkWords) / float64(nwords)
	score := linkDivisor * float64(nlinks)

	if score > 1.0 {
		return true
	}
	return false
}

func (this *contentExtractor) isTableAndNoParaExist(selection *goquery.Selection) bool {
	subParagraph := selection.Find("p")
	subParagraph.Each(func(i int, s *goquery.Selection) {
		txt := s.Text()
		if len(txt) < 25 {
			node := s.Get(0)
			parent := node.Parent
			parent.RemoveChild(node)
		}
	})

	subParagraph2 := selection.Find("p")
	if subParagraph2.Length() == 0 && selection.Get(0).DataAtom.String() != "td" {
		return true
	}
	return false
}

func (this *contentExtractor) isNodescoreThresholdMet(node *goquery.Selection, e *goquery.Selection) bool {
	topNodeScore := this.getNodeGravityScore(node)
	currentNodeScore := this.getNodeGravityScore(e)
	threasholdScore := float64(topNodeScore) * 0.08
	if (float64(currentNodeScore) < threasholdScore) && e.Get(0).DataAtom.String() != "td" {
		return false
	}
	return true
}

func (this *contentExtractor) getSiblingsScore(topNode *goquery.Selection) int {
	base := 100000
	paragraphNumber := 0
	paragraphScore := 0
	nodesToCheck := topNode.Find("p")
	nodesToCheck.Each(func(i int, s *goquery.Selection) {
		textNode := s.Text()
		ws := this.config.stopWords.stopWordsCount(this.config.targetLanguage, textNode)
		highLinkDensity := this.isHighLinkDensity(s)
		if ws.stopWordCount > 2 && !highLinkDensity {
			paragraphNumber++
			paragraphScore += ws.stopWordCount
		}
	})
	if paragraphNumber > 0 {
		base = paragraphScore / paragraphNumber
	}
	return base
}

func (this *contentExtractor) getSiblingsContent(currentSibling *goquery.Selection, baselinescoreSiblingsPara float64) []*goquery.Selection {
	ps := make([]*goquery.Selection, 0)
	if currentSibling.Get(0).DataAtom.String() == "p" && len(currentSibling.Text()) > 0 {
		ps = append(ps, currentSibling)
		return ps
	} else {
		potentialParagraphs := currentSibling.Find("p")
		potentialParagraphs.Each(func(i int, s *goquery.Selection) {
			text := s.Text()
			if len(text) > 0 {
				ws := this.config.stopWords.stopWordsCount(this.config.targetLanguage, text)
				paragraphScore := ws.stopWordCount
				siblingBaselineScore := 0.30
				highLinkDensity := this.isHighLinkDensity(s)
				score := siblingBaselineScore * baselinescoreSiblingsPara
				if score < float64(paragraphScore) && !highLinkDensity {
					node := new(html.Node)
					node.Type = html.TextNode
					node.Data = text
					node.DataAtom = atom.P
					nodes := make([]*html.Node, 1)
					nodes[0] = node
					newSelection := new(goquery.Selection)
					newSelection.Nodes = nodes
					ps = append(ps, newSelection)
				}
			}

		})
	}
	return ps
}

func (this *contentExtractor) walkSiblings(node *goquery.Selection) []*goquery.Selection {
	currentSibling := node.Prev()
	b := make([]*goquery.Selection, 0)
	for currentSibling.Length() != 0 {
		b = append(b, currentSibling)
		previousSibling := currentSibling.Prev()
		currentSibling = previousSibling
	}
	return b
}

func (this *contentExtractor) addSiblings(topNode *goquery.Selection) *goquery.Selection {
	baselinescoreSiblingsPara := this.getSiblingsScore(topNode)
	results := this.walkSiblings(topNode)
	for _, currentNode := range results {
		ps := this.getSiblingsContent(currentNode, float64(baselinescoreSiblingsPara))
		for _, p := range ps {
			nodes := make([]*html.Node, len(topNode.Nodes)+1)
			nodes[0] = p.Get(0)
			for i, node := range topNode.Nodes {
				nodes[i+1] = node
			}
			topNode.Nodes = nodes
		}
	}
	return topNode
}

func (this *contentExtractor) postCleanup(targetNode *goquery.Selection) *goquery.Selection {
	node := this.addSiblings(targetNode)
	children := node.Children()
	children.Each(func(i int, s *goquery.Selection) {
		sTag := s.Get(0).DataAtom.String()
		if sTag != "p" {
			if this.isHighLinkDensity(s) || this.isTableAndNoParaExist(s) || !this.isNodescoreThresholdMet(node, s) {
				sNode := s.Get(0)
				sNode.Parent.RemoveChild(sNode)
			}

		}
	})
	return node
}

func (this *contentExtractor) setAttr(selection *goquery.Selection, attr string, value string) {
	node := selection.Get(0)

	for _, a := range node.Attr {
		if a.Key == attr {
			a.Val = value
			return
		}
	}
	attrs := make([]html.Attribute, len(node.Attr)+1)
	for i, a := range node.Attr {
		attrs[i+1] = a
	}
	newAttr := new(html.Attribute)
	newAttr.Key = attr
	newAttr.Val = value
	attrs[0] = *newAttr
	node.Attr = attrs
}
