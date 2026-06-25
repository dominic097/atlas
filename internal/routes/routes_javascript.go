package routes

// jsRoutes extracts producer routes + consumer calls from a JavaScript/
// TypeScript source file. Stubbed: a follow-up workflow fills in the
// express/fastify/nest producer patterns and the fetch/axios/apiClient consumer
// patterns. Returning nil keeps the ExtractFile dispatch compiling today.
func jsRoutes(filePath, content string) []RawRoute { return nil }
