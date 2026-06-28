const { execSync } = require('child_process');
const fs = require('fs');

try {
  console.log('Exporting users to CSV...');
  const cmd = `docker-compose exec -T postgres psql -U lapangango_user -d lapangango_db -c "COPY (SELECT id, name, email, role, phone, status, created_at FROM users) TO STDOUT WITH CSV HEADER"`;
  
  const result = execSync(cmd, { stdio: ['pipe', 'pipe', 'pipe'], encoding: 'utf8' });
  fs.writeFileSync('Data_Pengguna.csv', result);
  console.log('✅ Export successful. Saved to Data_Pengguna.csv');
} catch (error) {
  console.error('Export failed:', error.stderr || error.message);
}
