#!/usr/bin/env node

import { execSync } from 'child_process';
import { writeFileSync, readFileSync, existsSync, statSync, mkdirSync } from 'fs';
import { join, dirname } from 'path';

/**
 * Generate TypeScript types from Go DTOs by parsing Go struct definitions
 * Watches for changes in Go files and regenerates automatically
 */

const GO_MODELS_PATH = 'internal/db/';
const GO_PARSER_PATH = 'internal/parser/';
const TS_OUTPUT_PATH = 'frontend/src/types/generated.ts';

// Helper function to convert Go types to TypeScript
function convertGoTypeToTS(goType) {
  const cleanType = goType.trim().replace(/^\*/, ''); // Remove pointer indicator
  
  // Handle Go slices: []Type -> Type[]
  if (cleanType.startsWith('[]')) {
    const elementType = cleanType.slice(2); // Remove []
    return convertGoTypeToTS(elementType) + '[]';
  }
  
  // Handle sql.Null types
  if (cleanType.includes('sql.NullString')) return 'string | null';
  if (cleanType.includes('sql.NullInt64')) return 'number | null';
  if (cleanType.includes('sql.NullTime')) return 'string | null';
  if (cleanType.includes('sql.NullBool')) return 'boolean | null';
  if (cleanType.includes('sql.NullFloat64')) return 'number | null';
  
  // Handle Go package prefixes
  if (cleanType.includes('*db.History')) return 'History | null';
  if (cleanType.includes('db.History')) return 'History';
  
  if (cleanType.includes('time.Time')) return 'string'; // ISO date strings
  if (cleanType.includes('time.Duration')) return 'number'; // Nanoseconds
  if (cleanType.includes('map[string]interface{}')) return 'Record<string, unknown>';
  if (cleanType.includes('map[string]string')) return 'Record<string, string>';
  if (cleanType.includes('map[string]any')) return 'Record<string, unknown>';
  if (cleanType.includes('string')) return 'string';
  if (cleanType.includes('int') || cleanType.includes('float')) return 'number';
  if (cleanType.includes('bool')) return 'boolean';
  if (cleanType.match(/^[A-Z][a-zA-Z0-9]*$/)) return cleanType; // Custom struct types
  
  return 'unknown'; // Fallback
}

// Parse Go struct to extract JSON field names and types
function parseGoStruct(goCode, structName) {
  // More sophisticated regex to handle nested braces in type definitions
  const structStart = new RegExp(`type\\s+${structName}\\s+struct\\s*{`, 's');
  const startMatch = goCode.match(structStart);
  
  if (!startMatch) return null;
  
  // Find the matching closing brace by counting braces
  const startIndex = startMatch.index + startMatch[0].length;
  let braceCount = 1;
  let endIndex = startIndex;
  
  for (let i = startIndex; i < goCode.length && braceCount > 0; i++) {
    if (goCode[i] === '{') braceCount++;
    if (goCode[i] === '}') braceCount--;
    endIndex = i;
  }
  
  if (braceCount !== 0) return null; // Unmatched braces
  
  const structContent = goCode.substring(startIndex, endIndex);
  const match = [null, structContent]; // Simulate regex match format
  
  if (!match) return null;
  
  // First, reconstruct the struct content by merging lines that belong together
  const rawContent = match[1];
  const lines = rawContent.split('\n').map(line => line.trim()).filter(line => line && !line.startsWith('//'));
  
  // Merge multi-line field definitions
  const mergedFields = [];
  let currentField = '';
  
  for (const line of lines) {
    // If line starts with a field name (capital letter), it's a new field
    if (/^\w+\s+/.test(line)) {
      // Save previous field if exists
      if (currentField) {
        mergedFields.push(currentField);
      }
      currentField = line;
    } else {
      // This line continues the previous field
      currentField += ' ' + line;
    }
  }
  
  // Don't forget the last field
  if (currentField) {
    mergedFields.push(currentField);
  }
  
  const fields = mergedFields.map(fieldLine => {
    // Check for embedded struct (no field name, just type)
    const embeddedMatch = fieldLine.match(/^([A-Z][a-zA-Z0-9]*)\s*$/);
    if (embeddedMatch) {
      const embeddedType = embeddedMatch[1];
      // Return a special marker for embedded structs
      return {
        embedded: true,
        type: embeddedType
      };
    }
    
    // Parse: FieldName type `json:"field_name,omitempty"`
    const fieldMatch = fieldLine.match(/^(\w+)\s+([^`]+?)\s*`json:"([^"]+)".*?`/);
    if (!fieldMatch) return null;
    
    const [, fieldName, goType, jsonTag] = fieldMatch;
    const jsonField = jsonTag.split(',')[0]; // Remove omitempty, etc.
    const isOptional = jsonTag.includes('omitempty') || jsonTag.includes(',omitempty') || goType.trim().startsWith('*');
    
    // Convert Go types to TypeScript using helper function
    const tsType = convertGoTypeToTS(goType.trim());
    
    return {
      name: jsonField,
      type: tsType,
      optional: isOptional
    };
  })
  .filter(Boolean);
  
  // Handle embedded structs by resolving their fields
  const resolvedFields = [];
  for (const field of fields) {
    if (field.embedded) {
      // Find the embedded struct and get its fields
      const embeddedFields = parseGoStruct(goCode, field.type);
      if (embeddedFields) {
        resolvedFields.push(...embeddedFields);
      }
    } else {
      resolvedFields.push(field);
    }
  }
  
  return resolvedFields;
}

function generateTSInterface(structName, fields) {
  const interfaceFields = fields.map(field => 
    `  ${field.name}${field.optional ? '?' : ''}: ${field.type};`
  ).join('\n');
  
  return `export interface ${structName} {\n${interfaceFields}\n}`;
}

// Function to discover all struct names in Go code
function findAllStructs(goCode) {
  const structRegex = /type\s+([A-Z][a-zA-Z0-9]*)\s+struct\s*{/g;
  const structs = [];
  let match;
  
  while ((match = structRegex.exec(goCode)) !== null) {
    structs.push(match[1]);
  }
  
  return structs;
}

// Function to find Go enums (const declarations)
function findGoEnums(goCode) {
  const enums = [];
  
  // Look for InputType specifically
  if (goCode.includes('type InputType int')) {
    // Find the const block that contains InputType values
    const constRegex = /const\s*\(\s*([\s\S]*?)\)/;
    const matches = goCode.match(constRegex);
    
    if (matches) {
      const constBody = matches[1];
      // Look for any identifier that starts with InputType
      const lines = constBody.split('\n');
      const values = [];
      let foundInputTypeBlock = false;
      
      for (const line of lines) {
        const trimmedLine = line.trim();
        
        // Skip comment lines
        if (trimmedLine.startsWith('//')) continue;
        
        // Check if this line contains an InputType identifier
        const identifierMatch = trimmedLine.match(/^\s*(InputType\w+)/);
        if (identifierMatch) {
          values.push(identifierMatch[1]);
          foundInputTypeBlock = true;
        } else if (foundInputTypeBlock && trimmedLine === ')') {
          // End of const block
          break;
        }
      }
      
      if (values.length > 0) {
        enums.push({ name: 'InputType', values });
      }
    }
  }
  
  return enums;
}

function generateTSEnum(enumName, values) {
  const enumValues = values.map((value, index) => `  ${value} = ${index}`).join(',\n');
  return `export enum ${enumName} {\n${enumValues}\n}`;
}

function generateTypes() {
  console.log('🔧 Parsing Go structs and generating TypeScript types...');
  
  const sourceFiles = [
    { path: GO_MODELS_PATH, files: ['models.go'] },
    { path: GO_PARSER_PATH, files: ['types.go'] }
  ];
  
  let combinedTypes = `// GENERATED FROM GO FILES: SEE THE GENERATOR SCRIPT
// Auto-generated TypeScript types from Go structs
// Generated on: ${new Date().toISOString()}
// Generator script: scripts/generate-types.js
// Source files: internal/db/models.go, internal/parser/types.go
// Do not edit manually - run 'node scripts/generate-types.js' to regenerate

`;

  for (const { path, files } of sourceFiles) {
    for (const filename of files) {
      const filePath = join(path, filename);
      if (!existsSync(filePath)) continue;
      
      const goCode = readFileSync(filePath, 'utf-8');
      console.log(`  📄 Processing ${filename}...`);
      
      // Automatically discover all structs in this file
      const structsInFile = findAllStructs(goCode);
      const enumsInFile = findGoEnums(goCode);
      
      // Generate enums first
      console.log(`    Found ${enumsInFile.length} enums in ${filename}`);
      for (const enumDef of enumsInFile) {
        const tsEnum = generateTSEnum(enumDef.name, enumDef.values);
        combinedTypes += tsEnum + '\n\n';
        console.log(`    ✓ Generated enum ${enumDef.name} with values: ${enumDef.values.join(', ')}`);
      }
      
      for (const structName of structsInFile) {
        const fields = parseGoStruct(goCode, structName);
        if (fields && fields.length > 0) {
          const tsInterface = generateTSInterface(structName, fields);
          combinedTypes += tsInterface + '\n\n';
          console.log(`    ✓ Generated ${structName}`);
        }
      }
    }
  }
  
  // Add manual types that are hard to auto-generate
  combinedTypes += `// Manual types and enums
export enum InputType {
  InputTypeDirectURL = 0,
  InputTypeSearchShortcut = 1,
  InputTypeHistorySearch = 2,
  InputTypeFallbackSearch = 3
}

// Type aliases for frontend usage
export type HistoryEntry = History;
export type SearchShortcut = Shortcut;
export type ZoomEntry = ZoomSetting;
`;
  
  // Ensure output directory exists
  mkdirSync(dirname(TS_OUTPUT_PATH), { recursive: true });
  
  writeFileSync(TS_OUTPUT_PATH, combinedTypes);
  console.log(`✅ TypeScript types generated at: ${TS_OUTPUT_PATH}`);
}

// One-time generation
generateTypes();