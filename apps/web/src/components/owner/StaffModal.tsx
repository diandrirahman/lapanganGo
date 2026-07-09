import React, { useState, useEffect } from 'react';
import { X, Check } from 'lucide-react';
import type { StaffMember, CreateStaffRequest, UpdateStaffRequest } from '../../types/staff';
import { STAFF_PERMISSIONS } from '../../types/staff';
import type { Venue } from '../../types/venue';

interface StaffModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSubmit: (data: CreateStaffRequest | UpdateStaffRequest) => Promise<void>;
  initialData?: StaffMember | null;
  venues: Venue[];
}

export const StaffModal: React.FC<StaffModalProps> = ({
  isOpen,
  onClose,
  onSubmit,
  initialData,
  venues,
}) => {
  const [formData, setFormData] = useState<CreateStaffRequest>({
    name: '',
    email: '',
    phone: '',
    password: '',
    role: 'MANAGER',
    permissions: [],
    venue_ids: [],
  });
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    if (initialData) {
      setFormData({
        name: initialData.name,
        email: initialData.email,
        phone: initialData.phone || '',
        password: '', // Password is not returned and not updated here
        role: initialData.role,
        permissions: initialData.permissions || [],
        venue_ids: initialData.venue_ids || [],
      });
    } else {
      setFormData({
        name: '',
        email: '',
        phone: '',
        password: '',
        role: 'MANAGER',
        permissions: [],
        venue_ids: [],
      });
    }
  }, [initialData, isOpen]);

  if (!isOpen) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setIsSubmitting(true);
    try {
      if (initialData) {
        const updateData: UpdateStaffRequest = {
          name: formData.name,
          phone: formData.phone || undefined,
          role: formData.role,
          permissions: formData.permissions,
          venue_ids: formData.venue_ids || [],
        };
        await onSubmit(updateData);
      } else {
        const createData: CreateStaffRequest = {
          name: formData.name,
          email: formData.email,
          password: formData.password,
          phone: formData.phone || undefined,
          role: formData.role,
          permissions: formData.permissions,
          venue_ids: formData.venue_ids || [],
        };
        await onSubmit(createData);
      }
      onClose();
    } catch (error) {
      console.error(error);
    } finally {
      setIsSubmitting(false);
    }
  };

  const togglePermission = (permId: string) => {
    setFormData(prev => ({
      ...prev,
      permissions: prev.permissions.includes(permId)
        ? prev.permissions.filter(p => p !== permId)
        : [...prev.permissions, permId]
    }));
  };

  const toggleVenue = (venueId: string) => {
    const current = formData.venue_ids || [];
    setFormData(prev => ({
      ...prev,
      venue_ids: current.includes(venueId)
        ? current.filter(id => id !== venueId)
        : [...current, venueId]
    }));
  };

  const groupedPermissions = STAFF_PERMISSIONS.reduce((acc, perm) => {
    if (!acc[perm.category]) {
      acc[perm.category] = [];
    }
    acc[perm.category].push(perm);
    return acc;
  }, {} as Record<string, typeof STAFF_PERMISSIONS>);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm">
      <div className="bg-white rounded-2xl shadow-xl w-full max-w-3xl flex flex-col max-h-[90vh]">
        <div className="flex justify-between items-center p-6 border-b border-gray-100 shrink-0">
          <h2 className="text-xl font-bold text-gray-900">
            {initialData ? 'Edit Staff' : 'Tambah Staff Baru'}
          </h2>
          <button onClick={onClose} className="p-2 text-gray-400 hover:text-gray-600 rounded-full hover:bg-gray-100 transition-colors">
            <X className="w-5 h-5" />
          </button>
        </div>

        <form onSubmit={handleSubmit} className="overflow-y-auto p-6 space-y-8">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            <div className="space-y-4">
              <h3 className="text-sm font-bold text-gray-900 uppercase tracking-wider">Informasi Dasar</h3>
              
              <div>
                <label className="block text-sm font-semibold text-gray-700 mb-1.5">Nama Lengkap *</label>
                <input
                  type="text"
                  required
                  value={formData.name}
                  onChange={e => setFormData({ ...formData, name: e.target.value })}
                  className="w-full px-4 py-2.5 rounded-xl border border-gray-200 focus:border-primary focus:ring-1 focus:ring-primary outline-none transition-all"
                  placeholder="Masukkan nama"
                />
              </div>

              {!initialData && (
                <>
                  <div>
                    <label className="block text-sm font-semibold text-gray-700 mb-1.5">Email *</label>
                    <input
                      type="email"
                      required
                      value={formData.email}
                      onChange={e => setFormData({ ...formData, email: e.target.value })}
                      className="w-full px-4 py-2.5 rounded-xl border border-gray-200 focus:border-primary focus:ring-1 focus:ring-primary outline-none transition-all"
                      placeholder="email@contoh.com"
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-semibold text-gray-700 mb-1.5">Password Sementara *</label>
                    <input
                      type="password"
                      required
                      minLength={8}
                      value={formData.password}
                      onChange={e => setFormData({ ...formData, password: e.target.value })}
                      className="w-full px-4 py-2.5 rounded-xl border border-gray-200 focus:border-primary focus:ring-1 focus:ring-primary outline-none transition-all"
                      placeholder="Minimal 8 karakter"
                    />
                  </div>
                </>
              )}

              <div>
                <label className="block text-sm font-semibold text-gray-700 mb-1.5">No. HP</label>
                <input
                  type="text"
                  value={formData.phone}
                  onChange={e => setFormData({ ...formData, phone: e.target.value })}
                  className="w-full px-4 py-2.5 rounded-xl border border-gray-200 focus:border-primary focus:ring-1 focus:ring-primary outline-none transition-all"
                  placeholder="08..."
                />
              </div>

              <div>
                <label className="block text-sm font-semibold text-gray-700 mb-1.5">Role *</label>
                <select
                  value={formData.role}
                  onChange={e => setFormData({ ...formData, role: e.target.value })}
                  className="w-full px-4 py-2.5 rounded-xl border border-gray-200 focus:border-primary focus:ring-1 focus:ring-primary outline-none transition-all bg-white"
                >
                  <option value="MANAGER">Manager</option>
                  <option value="CASHIER">Kasir</option>
                  <option value="OPERATIONS">Operasional</option>
                </select>
              </div>

              <div className="pt-2">
                <label className="block text-sm font-semibold text-gray-700 mb-2">Akses Venue</label>
                <p className="text-xs text-gray-500 mb-3">Jika dikosongkan, staff tidak dapat melihat data venue mana pun.</p>
                <div className="space-y-2 max-h-48 overflow-y-auto pr-2">
                  {venues.map(v => (
                    <label key={v.id} className="flex items-start gap-3 p-3 rounded-xl border border-gray-100 hover:bg-gray-50 cursor-pointer transition-colors">
                      <div className={`mt-0.5 flex shrink-0 items-center justify-center w-5 h-5 rounded border ${
                        formData.venue_ids?.includes(v.id) ? 'bg-primary border-primary text-white' : 'border-gray-300'
                      }`}>
                        {formData.venue_ids?.includes(v.id) && <Check className="w-3.5 h-3.5" />}
                      </div>
                      <input
                        type="checkbox"
                        className="sr-only"
                        checked={formData.venue_ids?.includes(v.id) || false}
                        onChange={() => toggleVenue(v.id)}
                      />
                      <span className="text-sm text-gray-700 font-medium">{v.name}</span>
                    </label>
                  ))}
                </div>
              </div>
            </div>

            <div className="space-y-4">
              <h3 className="text-sm font-bold text-gray-900 uppercase tracking-wider">Hak Akses (Permissions) *</h3>
              <div className="space-y-6 max-h-[60vh] overflow-y-auto pr-2">
                {Object.entries(groupedPermissions).map(([category, perms]) => (
                  <div key={category} className="space-y-3">
                    <h4 className="text-xs font-semibold text-gray-500 uppercase tracking-wider">{category}</h4>
                    <div className="space-y-2">
                      {perms.map(p => (
                        <label key={p.id} className="flex items-start gap-3 p-3 rounded-xl border border-gray-100 hover:bg-gray-50 cursor-pointer transition-colors">
                          <div className={`mt-0.5 flex shrink-0 items-center justify-center w-5 h-5 rounded border ${
                            formData.permissions.includes(p.id) ? 'bg-primary border-primary text-white' : 'border-gray-300'
                          }`}>
                            {formData.permissions.includes(p.id) && <Check className="w-3.5 h-3.5" />}
                          </div>
                          <input
                            type="checkbox"
                            className="sr-only"
                            checked={formData.permissions.includes(p.id)}
                            onChange={() => togglePermission(p.id)}
                          />
                          <span className="text-sm text-gray-700 font-medium">{p.label}</span>
                        </label>
                      ))}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </form>

        <div className="p-6 border-t border-gray-100 flex justify-end gap-3 shrink-0 bg-gray-50/50 rounded-b-2xl">
          <button
            type="button"
            onClick={onClose}
            className="px-6 py-2.5 rounded-xl font-bold text-gray-600 hover:bg-gray-100 transition-colors"
          >
            Batal
          </button>
          <button
            onClick={handleSubmit}
            disabled={isSubmitting || formData.permissions.length === 0}
            className="px-6 py-2.5 rounded-xl font-bold bg-primary text-white hover:bg-primary-dark transition-colors disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
          >
            {isSubmitting ? 'Menyimpan...' : 'Simpan'}
          </button>
        </div>
      </div>
    </div>
  );
};
