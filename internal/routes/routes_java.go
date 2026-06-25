package routes

// javaRoutes extracts producer routes + consumer calls from a Java source file.
// Stubbed: a follow-up workflow fills in the Spring @RequestMapping/@GetMapping
// producer patterns and the RestTemplate/WebClient/HttpClient consumer
// patterns. Returning nil keeps the ExtractFile dispatch compiling today.
func javaRoutes(filePath, content string) []RawRoute { return nil }
