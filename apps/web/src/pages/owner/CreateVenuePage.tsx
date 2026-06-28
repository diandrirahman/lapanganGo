import React, { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import toast from 'react-hot-toast';
import { PageShell } from '../../components/layout/PageShell';
import { useAuth } from '../../contexts/AuthContext';
import { createOwnerVenue, fetchFacilities } from '../../lib/api';
import type { Facility } from '../../types/venue';
import { Building, MapPin, AlignLeft, Info, Grid } from 'lucide-react';

export const CreateVenuePage: React.FC = () => {
  const { token } = useAuth();
  const navigate = useNavigate();
  const [facilities, setFacilities] = useState<Facility[]>([]);

  useEffect(() => {
    fetchFacilities()
      .then(setFacilities)
      .catch(err => console.error('Failed to fetch facilities:', err));
  }, []);

  const [formData, setFormData] = useState({
    name: '',
    description: '',
    address: '',
    district: '',
    city: '',
    province: '',
    postal_code: '',
    latitude: '',
    longitude: '',
    facility_ids: [] as string[]
  });

  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token) return;

    try {
      setIsLoading(true);
      setError(null);
      const payload = {
        ...formData,
        latitude: formData.latitude !== '' ? parseFloat(formData.latitude as string) : undefined,
        longitude: formData.longitude !== '' ? parseFloat(formData.longitude as string) : undefined,
      };
      await createOwnerVenue(payload, token);
      toast.success('Venue berhasil ditambahkan!');
      navigate('/owner/venues');
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Terjadi kesalahan saat mendaftar venue.';
      setError(msg);
      toast.error(msg);
    } finally {
      setIsLoading(false);
    }
  };

  const handleChange = (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
    const { name, value } = e.target;
    setFormData(prev => ({ ...prev, [name]: value }));
  };

  const toggleFacility = (facilityId: string) => {
    setFormData(prev => {
      const exists = prev.facility_ids.includes(facilityId);
      if (exists) {
        return { ...prev, facility_ids: prev.facility_ids.filter(id => id !== facilityId) };
      }
      return { ...prev, facility_ids: [...prev.facility_ids, facilityId] };
    });
  };

  return (
    <PageShell>
      <div className="pt-24 pb-40 max-w-3xl mx-auto px-6">
        <div className="mb-10 text-center">
          <h1 className="text-3xl md:text-4xl font-extrabold text-text-main mb-4">
            Daftarkan Venue Baru
          </h1>
          <p className="text-text-muted">
            Isi formulir di bawah ini untuk menambahkan venue lapangan olahraga Anda ke sistem LapangGo.
          </p>
        </div>

        <div className="bg-white rounded-3xl p-8 shadow-sm border border-border-main">
          {error && (
            <div className="mb-6 p-4 bg-red-50 text-red-700 rounded-xl flex items-center gap-2 text-sm font-bold border border-red-100">
              <Info className="w-5 h-5 shrink-0" />
              {error}
            </div>
          )}

          <form onSubmit={handleSubmit} className="space-y-6">
            {/* Informasi Dasar */}
            <div>
              <h2 className="text-xl font-bold text-text-main mb-4 flex items-center gap-2">
                <Building className="w-5 h-5 text-primary" /> Informasi Dasar
              </h2>
              
              <div className="space-y-4">
                <div>
                  <label className="block text-sm font-bold text-text-main mb-1.5">Nama Venue *</label>
                  <input
                    type="text"
                    name="name"
                    required
                    value={formData.name}
                    onChange={handleChange}
                    className="w-full px-4 py-3 rounded-xl border border-border-main focus:border-primary focus:ring-1 focus:ring-primary outline-none"
                    placeholder="Contoh: Gelora Bung Karno"
                  />
                </div>
                <div>
                  <label className="block text-sm font-bold text-text-main mb-1.5 flex items-center gap-1.5">
                    <AlignLeft className="w-4 h-4 text-text-muted" /> Deskripsi
                  </label>
                  <textarea
                    name="description"
                    rows={3}
                    value={formData.description}
                    onChange={handleChange}
                    className="w-full px-4 py-3 rounded-xl border border-border-main focus:border-primary focus:ring-1 focus:ring-primary outline-none"
                    placeholder="Deskripsikan keunggulan venue Anda..."
                  />
                </div>
              </div>
            </div>

            <hr className="border-border-main" />

            {/* Fasilitas */}
            <div>
              <h2 className="text-xl font-bold text-text-main mb-4 flex items-center gap-2">
                <Grid className="w-5 h-5 text-primary" /> Fasilitas Venue
              </h2>
              <div className="grid grid-cols-2 md:grid-cols-3 gap-3">
                {facilities.map((fac) => {
                  const isSelected = formData.facility_ids.includes(fac.id);
                  return (
                    <label 
                      key={fac.id}
                      className={`flex items-center gap-2 p-3 rounded-xl cursor-pointer border transition-all ${isSelected ? 'border-primary bg-primary/5 text-primary' : 'border-border-main hover:border-border-heavy text-text-main'}`}
                    >
                      <input 
                        type="checkbox"
                        checked={isSelected}
                        onChange={() => toggleFacility(fac.id)}
                        className="rounded text-primary focus:ring-primary w-4 h-4 cursor-pointer"
                      />
                      <span className="text-sm font-medium">{fac.name}</span>
                    </label>
                  );
                })}
              </div>
            </div>

            <hr className="border-border-main" />

            {/* Lokasi */}
            <div>
              <h2 className="text-xl font-bold text-text-main mb-4 flex items-center gap-2">
                <MapPin className="w-5 h-5 text-primary" /> Lokasi Venue
              </h2>
              
              <div className="space-y-4">
                <div>
                  <label className="block text-sm font-bold text-text-main mb-1.5">Alamat Lengkap *</label>
                  <textarea
                    name="address"
                    required
                    rows={2}
                    value={formData.address}
                    onChange={handleChange}
                    className="w-full px-4 py-3 rounded-xl border border-border-main focus:border-primary focus:ring-1 focus:ring-primary outline-none"
                    placeholder="Jalan, Nomor, RT/RW"
                  />
                </div>
                
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm font-bold text-text-main mb-1.5">Kota / Kabupaten *</label>
                    <input
                      type="text"
                      name="city"
                      required
                      value={formData.city}
                      onChange={handleChange}
                      className="w-full px-4 py-3 rounded-xl border border-border-main focus:border-primary focus:ring-1 focus:ring-primary outline-none"
                      placeholder="Contoh: Jakarta Selatan"
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-bold text-text-main mb-1.5">Kecamatan</label>
                    <input
                      type="text"
                      name="district"
                      value={formData.district}
                      onChange={handleChange}
                      className="w-full px-4 py-3 rounded-xl border border-border-main focus:border-primary focus:ring-1 focus:ring-primary outline-none"
                      placeholder="Contoh: Kebayoran Baru"
                    />
                  </div>
                </div>

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div>
                    <label className="block text-sm font-bold text-text-main mb-1.5">Provinsi</label>
                    <input
                      type="text"
                      name="province"
                      value={formData.province}
                      onChange={handleChange}
                      className="w-full px-4 py-3 rounded-xl border border-border-main focus:border-primary focus:ring-1 focus:ring-primary outline-none"
                      placeholder="Contoh: DKI Jakarta"
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-bold text-text-main mb-1.5">Kode Pos</label>
                    <input
                      type="text"
                      name="postal_code"
                      value={formData.postal_code}
                      onChange={handleChange}
                      className="w-full px-4 py-3 rounded-xl border border-border-main focus:border-primary focus:ring-1 focus:ring-primary outline-none"
                      placeholder="Contoh: 12110"
                    />
                  </div>
                </div>
              </div>
            </div>

            <div className="pt-4 flex gap-4">
              <button
                type="button"
                onClick={() => navigate('/owner/venues')}
                className="flex-1 py-4 rounded-xl border-2 border-border-main text-text-main font-bold hover:bg-gray-50 transition-colors"
              >
                Batal
              </button>
              <button
                type="submit"
                disabled={isLoading}
                className="flex-1 py-4 rounded-xl bg-primary text-white font-bold hover:bg-primary/90 transition-colors disabled:opacity-50"
              >
                {isLoading ? 'Menyimpan...' : 'Simpan Venue'}
              </button>
            </div>
          </form>
        </div>
      </div>
    </PageShell>
  );
};
