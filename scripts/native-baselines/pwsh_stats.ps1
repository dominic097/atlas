param([Parameter(Mandatory=$true)][string]$root)
$ErrorActionPreference = "Stop"
$stats = [ordered]@{
  files = 0
  parsed_files = 0
  parse_errors = 0
  functions = 0
  assignments = 0
  definitions = 0
  powershell_version = $PSVersionTable.PSVersion.ToString()
  parse_error_samples = @()
}
Get-ChildItem -Path $root -Recurse -File -Include *.ps1,*.psm1,*.psd1 |
  Where-Object { $_.FullName -notmatch [regex]::Escape([IO.Path]::DirectorySeparatorChar + "graphify-out" + [IO.Path]::DirectorySeparatorChar) } |
  Sort-Object FullName |
  ForEach-Object {
    $stats.files++
    $tokens = $null
    $errors = $null
    $ast = [System.Management.Automation.Language.Parser]::ParseFile($_.FullName, [ref]$tokens, [ref]$errors)
    if ($errors -and $errors.Count) {
      $stats.parse_errors++
      if ($stats.parse_error_samples.Count -lt 8) {
        $stats.parse_error_samples += [ordered]@{
          path = [IO.Path]::GetRelativePath($root, $_.FullName)
          error = $errors[0].Message
        }
      }
    } else {
      $stats.parsed_files++
    }
    $stats.functions += @($ast.FindAll({ param($node) $node -is [System.Management.Automation.Language.FunctionDefinitionAst] }, $true)).Count
    $stats.assignments += @($ast.FindAll({ param($node) $node -is [System.Management.Automation.Language.AssignmentStatementAst] }, $true)).Count
  }
$stats.definitions = $stats.functions
$stats | ConvertTo-Json -Depth 8
