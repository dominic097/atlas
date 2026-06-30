#!/usr/bin/env php
<?php
declare(strict_types=1);

if ($argc !== 2) {
    fwrite(STDERR, "Usage: php_token_stats.php <root>\n");
    exit(2);
}

$root = realpath($argv[1]);
if ($root === false || !is_dir($root)) {
    fwrite(STDERR, "Root does not exist or is not a directory: {$argv[1]}\n");
    exit(2);
}

$skipTokenIds = array_filter([
    defined('T_WHITESPACE') ? T_WHITESPACE : null,
    defined('T_COMMENT') ? T_COMMENT : null,
    defined('T_DOC_COMMENT') ? T_DOC_COMMENT : null,
]);

$ampersandTokenIds = array_filter([
    defined('T_AMPERSAND_FOLLOWED_BY_VAR_OR_VARARG') ? T_AMPERSAND_FOLLOWED_BY_VAR_OR_VARARG : null,
    defined('T_AMPERSAND_NOT_FOLLOWED_BY_VAR_OR_VARARG') ? T_AMPERSAND_NOT_FOLLOWED_BY_VAR_OR_VARARG : null,
]);

$stats = [
    'files' => 0,
    'parsed_files' => 0,
    'parse_errors' => 0,
    'php_version' => PHP_VERSION,
    'definition_counts' => [
        'class' => 0,
        'interface' => 0,
        'trait' => 0,
        'enum' => 0,
        'function' => 0,
    ],
    'classes' => 0,
    'interfaces' => 0,
    'traits' => 0,
    'enums' => 0,
    'functions' => 0,
    'requires' => 0,
    'namespaces' => 0,
    'uses' => 0,
    'use_functions' => 0,
    'definitions' => 0,
    'sample_definitions' => [],
    'parse_error_samples' => [],
];

function token_id(mixed $token): ?int
{
    return is_array($token) ? $token[0] : null;
}

function token_text(mixed $token): string
{
    return is_array($token) ? $token[1] : (string) $token;
}

function is_skippable(mixed $token, array $skipTokenIds): bool
{
    $id = token_id($token);
    return $id !== null && in_array($id, $skipTokenIds, true);
}

function next_meaningful(array $tokens, int $index, array $skipTokenIds): ?int
{
    $count = count($tokens);
    for ($i = $index + 1; $i < $count; $i++) {
        if (!is_skippable($tokens[$i], $skipTokenIds)) {
            return $i;
        }
    }
    return null;
}

function previous_meaningful(array $tokens, int $index, array $skipTokenIds): ?int
{
    for ($i = $index - 1; $i >= 0; $i--) {
        if (!is_skippable($tokens[$i], $skipTokenIds)) {
            return $i;
        }
    }
    return null;
}

function identifier_text(mixed $token): ?string
{
    $text = token_text($token);
    return preg_match('/^[A-Za-z_][A-Za-z0-9_]*$/', $text) === 1 ? $text : null;
}

function add_definition(array &$stats, string $kind, string $name, string $path, string $root): void
{
    $stats['definition_counts'][$kind]++;
    if (count($stats['sample_definitions']) < 30) {
        $relative = str_starts_with($path, $root . DIRECTORY_SEPARATOR)
            ? substr($path, strlen($root) + 1)
            : $path;
        $stats['sample_definitions'][] = [
            'kind' => $kind,
            'name' => $name,
            'path' => $relative,
        ];
    }
}

function should_skip_path(string $path): bool
{
    $parts = explode(DIRECTORY_SEPARATOR, $path);
    return in_array('graphify-out', $parts, true);
}

function count_php_file(string $path, string $root, array &$stats, array $skipTokenIds, array $ampersandTokenIds): void
{
    $source = @file_get_contents($path);
    if ($source === false) {
        $stats['parse_errors']++;
        if (count($stats['parse_error_samples']) < 8) {
            $stats['parse_error_samples'][] = [
                'path' => $path,
                'error' => 'file_get_contents failed',
            ];
        }
        return;
    }

    try {
        $tokens = token_get_all($source, TOKEN_PARSE);
    } catch (ParseError $error) {
        $stats['parse_errors']++;
        if (count($stats['parse_error_samples']) < 8) {
            $stats['parse_error_samples'][] = [
                'path' => $path,
                'error' => $error->getMessage(),
            ];
        }
        return;
    }

    $stats['parsed_files']++;
    $count = count($tokens);
    for ($i = 0; $i < $count; $i++) {
        $token = $tokens[$i];
        $id = token_id($token);
        if ($id === null) {
            continue;
        }

        if ($id === T_CLASS || $id === T_INTERFACE || $id === T_TRAIT || (defined('T_ENUM') && $id === T_ENUM)) {
            if ($id === T_CLASS) {
                $previous = previous_meaningful($tokens, $i, $skipTokenIds);
                if ($previous !== null && token_id($tokens[$previous]) === T_DOUBLE_COLON) {
                    continue;
                }
            }
            $next = next_meaningful($tokens, $i, $skipTokenIds);
            if ($next === null) {
                continue;
            }
            $name = token_id($tokens[$next]) === T_STRING ? token_text($tokens[$next]) : null;
            if ($name === null) {
                continue;
            }

            $kind = match ($id) {
                T_CLASS => 'class',
                T_INTERFACE => 'interface',
                T_TRAIT => 'trait',
                default => 'enum',
            };
            add_definition($stats, $kind, $name, $path, $root);
            continue;
        }

        if ($id === T_FUNCTION) {
            $previous = previous_meaningful($tokens, $i, $skipTokenIds);
            if ($previous !== null && token_id($tokens[$previous]) === T_USE) {
                continue;
            }
            $nameIndex = next_meaningful($tokens, $i, $skipTokenIds);
            if ($nameIndex !== null) {
                $nameTokenId = token_id($tokens[$nameIndex]);
                if (
                    token_text($tokens[$nameIndex]) === '&' ||
                    ($nameTokenId !== null && in_array($nameTokenId, $ampersandTokenIds, true))
                ) {
                    $nameIndex = next_meaningful($tokens, $nameIndex, $skipTokenIds);
                }
            }
            if ($nameIndex === null) {
                continue;
            }
            $name = identifier_text($tokens[$nameIndex]);
            if ($name === null) {
                continue;
            }
            $parenIndex = next_meaningful($tokens, $nameIndex, $skipTokenIds);
            if ($parenIndex === null || token_text($tokens[$parenIndex]) !== '(') {
                continue;
            }
            add_definition($stats, 'function', $name, $path, $root);
            continue;
        }

        if (in_array($id, [T_REQUIRE, T_REQUIRE_ONCE, T_INCLUDE, T_INCLUDE_ONCE], true)) {
            $stats['requires']++;
        } elseif ($id === T_NAMESPACE) {
            $stats['namespaces']++;
        } elseif ($id === T_USE) {
            $stats['uses']++;
            $next = next_meaningful($tokens, $i, $skipTokenIds);
            if ($next !== null && token_id($tokens[$next]) === T_FUNCTION) {
                $stats['use_functions']++;
            }
        }
    }
}

$iterator = new RecursiveIteratorIterator(
    new RecursiveDirectoryIterator($root, FilesystemIterator::SKIP_DOTS)
);

$paths = [];
foreach ($iterator as $entry) {
    if (!$entry->isFile()) {
        continue;
    }
    $path = $entry->getPathname();
    if (should_skip_path($path)) {
        continue;
    }
    if (strtolower($entry->getExtension()) === 'php') {
        $paths[] = $path;
    }
}
sort($paths, SORT_STRING);

foreach ($paths as $path) {
    $stats['files']++;
    count_php_file($path, $root, $stats, $skipTokenIds, $ampersandTokenIds);
}

$stats['classes'] = $stats['definition_counts']['class'];
$stats['interfaces'] = $stats['definition_counts']['interface'];
$stats['traits'] = $stats['definition_counts']['trait'];
$stats['enums'] = $stats['definition_counts']['enum'];
$stats['functions'] = $stats['definition_counts']['function'];
$stats['definitions'] = array_sum($stats['definition_counts']);

echo json_encode($stats, JSON_PRETTY_PRINT | JSON_UNESCAPED_SLASHES) . "\n";
