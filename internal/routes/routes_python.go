package routes

// pythonRoutes extracts producer routes + consumer calls from a Python source
// file. Stubbed: a follow-up workflow fills in the flask/fastapi/django
// producer patterns and the requests/httpx consumer patterns. Returning nil
// keeps the ExtractFile dispatch compiling today.
func pythonRoutes(filePath, content string) []RawRoute { return nil }
