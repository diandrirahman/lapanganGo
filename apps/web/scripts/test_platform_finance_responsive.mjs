import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

// This script simulates a test that ensures the platform finance components
// use standard CSS media queries for responsiveness.
function checkResponsiveClasses() {
  const adminFinancePagePath = path.join(__dirname, '..', 'src', 'pages', 'admin', 'AdminPlatformFinancePage.tsx');
  
  if (!fs.existsSync(adminFinancePagePath)) {
    console.error('File not found:', adminFinancePagePath);
    process.exit(1);
  }

  const content = fs.readFileSync(adminFinancePagePath, 'utf8');
  
  // Basic heuristic: verify there is usage of md: flex-col vs flex-row or block vs flex 
  // representing responsive grid/layout adaptation.
  if (!content.includes('md:') && !content.includes('lg:') && !content.includes('sm:')) {
    console.error('FAILED: AdminPlatformFinancePage.tsx does not seem to contain Tailwind responsive prefixes (md:, lg: etc).');
    process.exit(1);
  }

  console.log('SUCCESS: Platform finance module passes basic responsive static check.');
}

checkResponsiveClasses();
