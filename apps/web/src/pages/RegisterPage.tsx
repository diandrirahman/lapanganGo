import React, { useState } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import toast from 'react-hot-toast';
import { PageShell } from '../components/layout/PageShell';
import { Input } from '../components/ui/Input';
import { Button } from '../components/ui/Button';
import { AlertCircle } from 'lucide-react';

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080';

export const RegisterPage: React.FC = () => {
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [phone, setPhone] = useState('');
  const [password, setPassword] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  
  const navigate = useNavigate();

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsLoading(true);
    setError(null);

    try {
      // Mock mode fallback
      if (import.meta.env.VITE_USE_MOCK_AUTH === 'true') {
        setTimeout(() => {
          toast.success('Pendaftaran berhasil! Silakan login.');
          navigate('/login');
        }, 1000);
        return;
      }

      const response = await fetch(`${API_BASE_URL}/auth/register`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, email, phone, password })
      });

      const data = await response.json();

      if (!response.ok) {
        throw new Error(data.message || data.error || 'Pendaftaran gagal. Silakan periksa kembali data Anda.');
      }

      // Automatically redirect to login page after successful registration
      toast.success('Pendaftaran berhasil! Silakan login.');
      navigate('/login');
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Terjadi kesalahan';
      setError(msg);
      toast.error(msg);
      setIsLoading(false);
    }
  };

  return (
    <PageShell>
      <div className="min-h-[85vh] flex items-center justify-center py-12 px-6">
        <div className="w-full max-w-[900px] bg-surface rounded-3xl shadow-float border border-border-main overflow-hidden flex flex-col md:flex-row">
          
          {/* Left Side - Branding */}
          <div className="md:w-5/12 bg-primary-gradient p-12 text-white flex flex-col justify-between relative overflow-hidden hidden md:flex">
            
            <div className="relative z-10">
              <h2 className="text-3xl font-extrabold mb-4 leading-tight">Mulai<br/>Petualangan<br/>Barumu!</h2>
              <p className="text-white/80 font-medium text-sm leading-relaxed">
                Bergabung dengan komunitas olahraga terbesar dan temukan lawan tanding sepadan di sekitarmu.
              </p>
            </div>
            
            <div className="relative z-10">
              <div className="flex -space-x-3 mb-3">
                {[1,2,3,4].map(i => (
                  <div key={i} className="w-8 h-8 rounded-full border-2 border-[#FF512F] bg-white/20 backdrop-blur-sm flex items-center justify-center text-[10px] font-bold">
                    User
                  </div>
                ))}
              </div>
              <p className="text-xs font-medium text-white/90">Jadilah bagian dari revolusi olahraga</p>
            </div>
          </div>

          {/* Right Side - Form */}
          <div className="md:w-7/12 p-8 md:p-12 lg:p-16 flex flex-col justify-center bg-surface">
            <div className="mb-8">
              <h1 className="text-3xl font-extrabold tracking-tight text-text-main mb-2">Daftar Akun Baru</h1>
              <p className="text-text-muted font-medium">Lengkapi profil Anda untuk mulai booking lapangan</p>
            </div>

            {error && (
              <div className="bg-red-50/80 backdrop-blur-sm text-red-600 p-4 rounded-2xl flex items-start gap-3 mb-6 text-[14px] font-bold border border-red-100">
                <AlertCircle className="w-5 h-5 shrink-0 mt-0.5" />
                <p>{error}</p>
              </div>
            )}

            <form onSubmit={handleSubmit} className="space-y-4">
              <Input
                label="Nama Lengkap"
                type="text"
                placeholder="Bima Aditya"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
              />
              <Input
                label="Alamat Email"
                type="email"
                placeholder="nama@email.com"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
              />
              <Input
                label="Nomor WhatsApp"
                type="tel"
                placeholder="081234567890"
                value={phone}
                onChange={(e) => setPhone(e.target.value)}
              />
              <Input
                label="Kata Sandi"
                type="password"
                placeholder="********"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
              />
              
              <div className="pt-4">
                <Button 
                  type="submit" 
                  className="w-full py-3.5 rounded-xl text-base shadow-primary-glow" 
                  isLoading={isLoading}
                >
                  Daftar Sekarang
                </Button>
              </div>
            </form>

            <p className="text-center text-[14px] text-text-muted mt-8 font-medium">
              Sudah punya akun? <Link to="/login" className="text-primary font-bold hover:underline transition-all">Masuk di sini</Link>
            </p>
          </div>

        </div>
      </div>
    </PageShell>
  );
};
