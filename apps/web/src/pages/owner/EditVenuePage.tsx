import React, { useState, useEffect } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import toast from 'react-hot-toast';
import { PageShell } from '../../components/layout/PageShell';
import { useAuth } from '../../contexts/AuthContext';
import { 
  updateOwnerVenue, 
  fetchFacilities, 
  fetchOwnerVenueById, 
  addVenuePhoto, 
  deleteVenuePhoto, 
  updateVenuePhoto 
} from '../../lib/api';
import type { Facility, VenuePhoto } from '../../types/venue';
import { Building, MapPin, AlignLeft, Info, Grid, Image as ImageIcon, Trash2, Star, Upload } from 'lucide-react';
import { LoadingState } from '../../components/feedback/LoadingState';

import { SafeVenueImage } from '../../components/ui/SafeVenueImage';

// Helper component for fallback images
const GalleryPreviewImage = ({ photo, venueId, onSetPrimary, onDelete, isPhotoLoading }: { photo: VenuePhoto, venueId: string, onSetPrimary: (id: string) => void, onDelete: (id: string) => void, isPhotoLoading: boolean }) => {
  return (
    <div className={`relative rounded-xl overflow-hidden border-2 aspect-[4/3] group ${photo.is_primary ? 'border-primary ring-2 ring-primary/20' : 'border-border-main'}`}>
      <SafeVenueImage 
        src={photo.image_url}
        venueId={venueId}
        alt="Venue"
        className="w-full h-full bg-gray-100"
        fallbackIcon="image"
      />
      
      {photo.is_primary && (
        <div className="absolute top-2 left-2 bg-primary text-white text-[10px] font-extrabold px-2.5 py-1 rounded-full flex items-center gap-1 shadow-md">
          <Star className="w-3 h-3 fill-current" /> UTAMA
        </div>
      )}

      {/* Action overlay - keep it accessible on mobile by not strictly hiding it, or making it always visible gracefully */}
      <div className="absolute inset-x-0 bottom-0 p-2 bg-gradient-to-t from-black/60 to-transparent flex items-end justify-end gap-2 md:opacity-0 group-hover:opacity-100 transition-opacity">
        {!photo.is_primary && (
          <button 
            type="button"
            onClick={() => onSetPrimary(photo.id)}
            disabled={isPhotoLoading}
            className="p-2 bg-white text-text-main rounded-full hover:bg-primary hover:text-white transition-colors disabled:opacity-50"
            title="Jadikan Foto Utama"
          >
            <Star className="w-4 h-4" />
          </button>
        )}
        <button 
          type="button"
          onClick={() => onDelete(photo.id)}
          disabled={isPhotoLoading}
          className="p-2 bg-white text-red-500 rounded-full hover:bg-red-500 hover:text-white transition-colors disabled:opacity-50"
          title="Hapus Foto"
        >
          <Trash2 className="w-4 h-4" />
        </button>
      </div>
    </div>
  );
};

export const EditVenuePage: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const { token } = useAuth();
  const navigate = useNavigate();
  const [facilities, setFacilities] = useState<Facility[]>([]);
  
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

  const [photos, setPhotos] = useState<VenuePhoto[]>([]);
  const [newPhotoUrl, setNewPhotoUrl] = useState('');
  const [isPhotoLoading, setIsPhotoLoading] = useState(false);
  const [photoPreviewError, setPhotoPreviewError] = useState(false);

  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetchFacilities()
      .then(setFacilities)
      .catch(err => console.error('Failed to fetch facilities:', err));
  }, []);

  useEffect(() => {
    if (token && id) {
      fetchOwnerVenueById(id, token)
        .then(v => {
          setPhotos(v.photos || []);
          setFormData({
            name: v.name,
            description: v.description || '',
            address: v.address,
            district: v.district || '',
            city: v.city,
            province: v.province || '',
            postal_code: v.postal_code || '',
            latitude: v.latitude ? v.latitude.toString() : '',
            longitude: v.longitude ? v.longitude.toString() : '',
            facility_ids: v.facilities.map(f => f.id)
          });
        })
        .catch(err => {
          setError(err.message);
          toast.error('Gagal memuat data venue');
        })
        .finally(() => setIsLoading(false));
    }
  }, [id, token]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token || !id) return;

    try {
      setIsSaving(true);
      setError(null);
      const payload = {
        ...formData,
        latitude: formData.latitude !== '' ? parseFloat(formData.latitude as string) : undefined,
        longitude: formData.longitude !== '' ? parseFloat(formData.longitude as string) : undefined,
      };
      await updateOwnerVenue(id, payload, token);
      toast.success('Venue berhasil diperbarui!');
      navigate('/owner/venues');
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Terjadi kesalahan saat memperbarui venue.';
      setError(msg);
      toast.error(msg);
    } finally {
      setIsSaving(false);
    }
  };

  const handleAddPhoto = async (e: React.FormEvent) => {
    e.preventDefault();
    const url = newPhotoUrl.trim();
    if (!token || !id || !url) return;

    if (!url.startsWith('http://') && !url.startsWith('https://')) {
      toast.error('URL gambar harus berawalan http:// atau https://');
      return;
    }

    setIsPhotoLoading(true);

    const checkImageLoad = new Promise<boolean>((resolve) => {
      const img = new Image();
      img.onload = () => resolve(true);
      img.onerror = () => resolve(false);
      img.src = url;
      setTimeout(() => resolve(false), 10000);
    });

    const isImageValid = await checkImageLoad;
    
    if (!isImageValid) {
      toast.error('URL gambar tidak bisa dimuat langsung. Gunakan URL gambar direct yang dapat dibuka di browser.');
      setIsPhotoLoading(false);
      return;
    }

    try {
      const newPhoto = await addVenuePhoto(id, { image_url: url }, token);
      setPhotos(prev => [newPhoto, ...prev]);
      setNewPhotoUrl('');
      setPhotoPreviewError(false);
      toast.success('Foto berhasil ditambahkan');
    } catch (err: any) {
      toast.error(err.message || 'Gagal menambahkan foto');
    } finally {
      setIsPhotoLoading(false);
    }
  };

  const handleSetPrimaryPhoto = async (photoId: string) => {
    if (!token || !id) return;
    try {
      setIsPhotoLoading(true);
      await updateVenuePhoto(id, photoId, { is_primary: true }, token);
      
      // Update local state gracefully
      setPhotos(prev => prev.map(p => ({
        ...p,
        is_primary: p.id === photoId
      })));
      toast.success('Foto utama berhasil diubah');
    } catch (err: any) {
      toast.error(err.message || 'Gagal mengubah foto utama');
    } finally {
      setIsPhotoLoading(false);
    }
  };

  const handleDeletePhoto = async (photoId: string) => {
    if (!token || !id) return;
    if (!window.confirm('Yakin ingin menghapus foto ini?')) return;
    
    try {
      setIsPhotoLoading(true);
      await deleteVenuePhoto(id, photoId, token);
      setPhotos(prev => prev.filter(p => p.id !== photoId));
      toast.success('Foto berhasil dihapus');
    } catch (err: any) {
      toast.error(err.message || 'Gagal menghapus foto');
    } finally {
      setIsPhotoLoading(false);
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
        return { ...prev, facility_ids: prev.facility_ids.filter(fid => fid !== facilityId) };
      }
      return { ...prev, facility_ids: [...prev.facility_ids, facilityId] };
    });
  };

  if (isLoading) {
    return <PageShell><LoadingState message="Memuat detail venue..." /></PageShell>;
  }

  return (
    <PageShell>
      <div className="pt-24 pb-40 max-w-4xl mx-auto px-6">
        <div className="mb-10 flex flex-col md:flex-row md:items-center justify-between gap-4">
          <div>
            <h1 className="text-3xl md:text-4xl font-extrabold text-text-main mb-2">
              Edit Venue
            </h1>
            <p className="text-text-muted">
              Perbarui detail, fasilitas, dan kelola foto venue Anda.
            </p>
          </div>
          <button
            onClick={() => navigate('/owner/venues')}
            className="px-6 py-3 rounded-xl border-2 border-border-main text-text-main font-bold hover:bg-gray-50 transition-colors"
          >
            Kembali
          </button>
        </div>

        {error && (
          <div className="mb-6 p-4 bg-red-50 text-red-700 rounded-xl flex items-center gap-2 text-sm font-bold border border-red-100">
            <Info className="w-5 h-5 shrink-0" />
            {error}
          </div>
        )}

        <div className="space-y-8">
          {/* Main Form */}
          <form onSubmit={handleSubmit} className="bg-white rounded-2xl p-6 md:p-8 shadow-sm border border-border-main space-y-6">
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
                    />
                  </div>
                </div>
              </div>
            </div>

            <div className="pt-4">
              <button
                type="submit"
                disabled={isSaving}
                className="w-full py-4 rounded-xl bg-primary text-white font-bold hover:bg-primary/90 transition-colors disabled:opacity-50"
              >
                {isSaving ? 'Menyimpan Perubahan...' : 'Simpan Perubahan Data'}
              </button>
            </div>
          </form>

          {/* Kelola Foto */}
          <div className="bg-white rounded-2xl p-6 md:p-8 shadow-sm border border-border-main">
            <div className="mb-6">
              <h2 className="text-xl font-bold text-text-main mb-1 flex items-center gap-2">
                <ImageIcon className="w-5 h-5 text-primary" /> Kelola Foto Venue
              </h2>
              <p className="text-sm text-text-muted">Tambahkan URL foto, pilih foto utama, atau hapus foto.</p>
            </div>
            
            <form onSubmit={handleAddPhoto} className="mb-8">
              <label className="block text-sm font-bold text-text-main mb-1.5">Tambah URL Foto</label>
              <div className="flex flex-col sm:flex-row gap-3">
                <div className="flex-1">
                  <input
                    type="url"
                    required
                    placeholder="https://images.unsplash.com/..."
                    value={newPhotoUrl}
                    onChange={(e) => {
                      setNewPhotoUrl(e.target.value);
                      setPhotoPreviewError(false);
                    }}
                    className={`w-full px-4 py-3 rounded-xl border focus:ring-1 outline-none text-sm transition-colors ${photoPreviewError ? 'border-red-300 focus:border-red-500 focus:ring-red-500 bg-red-50' : 'border-border-main focus:border-primary focus:ring-primary'}`}
                  />
                  <p className="text-[11px] font-medium text-text-muted mt-1.5 ml-1">
                    Gunakan direct image URL yang bisa dibuka langsung, misalnya dari CDN yang mengizinkan hotlink.
                  </p>
                </div>
                <button 
                  type="submit"
                  disabled={isPhotoLoading || !newPhotoUrl || photoPreviewError}
                  className="sm:w-auto w-full px-6 py-3 bg-primary text-white font-bold rounded-xl text-sm disabled:opacity-50 hover:bg-primary/90 transition-colors h-[46px] shrink-0 flex items-center justify-center gap-2"
                >
                  <Upload className="w-4 h-4" />
                  {isPhotoLoading ? 'Menambahkan...' : 'Tambah Foto'}
                </button>
              </div>
              
              {newPhotoUrl && !newPhotoUrl.trim().includes(' ') && (
                <div className="mt-4 p-3 bg-gray-50 rounded-xl border border-gray-200 inline-block">
                  <p className="text-[11px] font-bold text-text-muted mb-2 uppercase tracking-wider">Preview</p>
                  <div className="w-32 h-24 rounded-lg overflow-hidden bg-gray-200 border border-border-main flex items-center justify-center">
                    <img 
                      src={newPhotoUrl.trim()} 
                      alt="Preview" 
                      className="w-full h-full object-cover"
                      onError={() => setPhotoPreviewError(true)}
                      onLoad={() => setPhotoPreviewError(false)}
                    />
                  </div>
                  {photoPreviewError && (
                    <p className="text-xs text-red-500 font-medium mt-2">Gambar tidak dapat dimuat.</p>
                  )}
                </div>
              )}
            </form>

            <div className="space-y-4">
              {photos.length === 0 ? (
                <div className="flex flex-col items-center justify-center py-16 px-4 bg-gray-50 rounded-2xl border-2 border-dashed border-gray-200">
                  <div className="w-16 h-16 bg-white rounded-full flex items-center justify-center shadow-sm mb-4">
                    <ImageIcon className="w-8 h-8 text-gray-400" />
                  </div>
                  <h3 className="text-base font-bold text-text-main mb-1">Belum ada foto venue</h3>
                  <p className="text-sm text-text-muted text-center max-w-sm">Tambahkan minimal 1 foto agar venue Anda terlihat menarik di halaman pencarian pelanggan.</p>
                </div>
              ) : (
                <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-4">
                  {photos.map(photo => (
                    <GalleryPreviewImage 
                      key={photo.id}
                      photo={photo}
                      venueId={id!}
                      onSetPrimary={handleSetPrimaryPhoto}
                      onDelete={handleDeletePhoto}
                      isPhotoLoading={isPhotoLoading}
                    />
                  ))}
                </div>
              )}
            </div>
          </div>
        </div>
      </div>
    </PageShell>
  );
};
