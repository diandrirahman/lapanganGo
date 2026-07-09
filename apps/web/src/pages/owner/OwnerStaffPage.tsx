import React, { useState, useEffect } from 'react';
import { PageShell } from '../../components/layout/PageShell';
import { useAuth } from '../../contexts/AuthContext';
import { Search, Shield, Edit2, Power, UserX, UserPlus } from 'lucide-react';
import { toast } from 'react-hot-toast';
import type { StaffMember, CreateStaffRequest, UpdateStaffRequest } from '../../types/staff';
import type { Venue } from '../../types/venue';
import { StaffModal } from '../../components/owner/StaffModal';

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || 'http://localhost:8080';

export const OwnerStaffPage: React.FC = () => {
  const { token, isActualOwner } = useAuth();
  const [staffList, setStaffList] = useState<StaffMember[]>([]);
  const [venues, setVenues] = useState<Venue[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [searchQuery, setSearchQuery] = useState('');
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [editingStaff, setEditingStaff] = useState<StaffMember | null>(null);

  const fetchData = async () => {
    try {
      const [staffRes, venuesRes] = await Promise.all([
        fetch(`${API_BASE_URL}/owner/staff`, {
          headers: { 'Authorization': `Bearer ${token}` }
        }),
        fetch(`${API_BASE_URL}/owner/venues`, {
          headers: { 'Authorization': `Bearer ${token}` }
        })
      ]);

      if (staffRes.ok) {
        const data = await staffRes.json();
        setStaffList(data.staff || []);
      }
      if (venuesRes.ok) {
        const data = await venuesRes.json();
        setVenues(data.venues || []);
      }
    } catch (error) {
      console.error('Failed to fetch data:', error);
      toast.error('Gagal memuat data staff');
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
  }, [token]);

  const handleCreate = async (data: CreateStaffRequest | UpdateStaffRequest) => {
    try {
      const res = await fetch(`${API_BASE_URL}/owner/staff`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json'
        },
        body: JSON.stringify(data)
      });
      
      const resData = await res.json();
      if (!res.ok) throw new Error(resData.message || 'Gagal menambah staff');
      
      toast.success('Staff berhasil ditambahkan');
      fetchData();
    } catch (error: any) {
      toast.error(error.message);
      throw error;
    }
  };

  const handleUpdate = async (id: string, data: CreateStaffRequest | UpdateStaffRequest) => {
    try {
      const res = await fetch(`${API_BASE_URL}/owner/staff/${id}`, {
        method: 'PUT',
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json'
        },
        body: JSON.stringify(data)
      });
      
      const resData = await res.json();
      if (!res.ok) throw new Error(resData.message || 'Gagal mengupdate staff');
      
      toast.success('Staff berhasil diupdate');
      fetchData();
    } catch (error: any) {
      toast.error(error.message);
      throw error;
    }
  };

  const handleModalSubmit = async (data: CreateStaffRequest | UpdateStaffRequest) => {
    if (editingStaff) {
      await handleUpdate(editingStaff.id, data);
    } else {
      await handleCreate(data);
    }
  };

  const openCreateModal = () => {
    setEditingStaff(null);
    setIsModalOpen(true);
  };

  const openEditModal = (staff: StaffMember) => {
    setEditingStaff(staff);
    setIsModalOpen(true);
  };

  const toggleStatus = async (staff: StaffMember) => {
    const newStatus = staff.status === 'ACTIVE' ? 'INACTIVE' : 'ACTIVE';
    try {
      const res = await fetch(`${API_BASE_URL}/owner/staff/${staff.id}/status`, {
        method: 'PATCH',
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({ status: newStatus })
      });
      
      if (!res.ok) throw new Error('Gagal mengubah status staff');
      
      toast.success(`Status staff berhasil diubah menjadi ${newStatus}`);
      fetchData();
    } catch (error: any) {
      toast.error(error.message);
    }
  };

  const filteredStaff = staffList.filter(s => 
    s.name.toLowerCase().includes(searchQuery.toLowerCase()) || 
    s.email.toLowerCase().includes(searchQuery.toLowerCase())
  );

  const getRoleLabel = (role: string) => {
    switch(role) {
      case 'MANAGER': return 'Manager';
      case 'CASHIER': return 'Kasir';
      case 'OPERATIONS': return 'Operasional';
      default: return role;
    }
  };

  if (!isActualOwner) {
    return (
      <PageShell>
        <div className="pt-32 pb-40 flex items-center justify-center">
          <div className="text-center">
            <Shield className="w-16 h-16 text-red-500 mx-auto mb-4" />
            <h1 className="text-2xl font-bold mb-2">Akses Ditolak</h1>
            <p className="text-gray-500">Hanya owner utama yang dapat mengelola staff.</p>
          </div>
        </div>
      </PageShell>
    );
  }

  return (
    <PageShell>
      <div className="pt-24 pb-40 max-w-6xl mx-auto px-4 md:px-6">
        <div className="flex flex-col md:flex-row justify-between items-start md:items-center gap-4 mb-8">
          <div>
            <h1 className="text-2xl md:text-3xl font-extrabold text-gray-900 tracking-tight">Kelola Staff</h1>
            <p className="text-gray-500 mt-1 font-medium">Beri akses tim Anda untuk mengelola operasional lapangan</p>
          </div>
          <button 
            onClick={openCreateModal}
            className="flex items-center gap-2 bg-primary text-white px-5 py-2.5 rounded-xl font-bold shadow-sm hover:shadow-md hover:-translate-y-1 transition-all"
          >
            <UserPlus className="w-5 h-5" />
            <span>Tambah Staff</span>
          </button>
        </div>

        <div className="bg-white rounded-[24px] shadow-sm border border-gray-100 overflow-hidden">
          <div className="p-4 md:p-6 border-b border-gray-100 flex flex-col md:flex-row gap-4 items-center justify-between bg-gray-50/50">
            <div className="relative w-full md:w-96">
              <Search className="w-5 h-5 text-gray-400 absolute left-4 top-1/2 -translate-y-1/2" />
              <input
                type="text"
                placeholder="Cari nama atau email..."
                value={searchQuery}
                onChange={e => setSearchQuery(e.target.value)}
                className="w-full pl-11 pr-4 py-2.5 rounded-xl border border-gray-200 focus:border-primary focus:ring-1 focus:ring-primary outline-none transition-all bg-white"
              />
            </div>
            <div className="text-sm font-semibold text-gray-500 bg-white px-4 py-2 rounded-xl border border-gray-200 shadow-sm">
              Total {filteredStaff.length} Staff
            </div>
          </div>

          <div className="overflow-x-auto">
            {isLoading ? (
              <div className="p-12 text-center text-gray-500 animate-pulse font-medium">Memuat data staff...</div>
            ) : filteredStaff.length === 0 ? (
              <div className="p-16 text-center flex flex-col items-center">
                <div className="w-16 h-16 bg-gray-50 rounded-2xl flex items-center justify-center mb-4 border border-gray-100">
                  <UserX className="w-8 h-8 text-gray-400" />
                </div>
                <h3 className="text-lg font-bold text-gray-900 mb-1">Belum ada staff</h3>
                <p className="text-gray-500 max-w-sm">Anda belum menambahkan staff ke workspace ini. Klik "Tambah Staff" untuk mengundang tim.</p>
              </div>
            ) : (
              <table className="w-full">
                <thead>
                  <tr className="border-b border-gray-100 bg-gray-50/50">
                    <th className="text-left py-4 px-6 text-xs font-bold text-gray-500 uppercase tracking-wider">Staff</th>
                    <th className="text-left py-4 px-6 text-xs font-bold text-gray-500 uppercase tracking-wider">Role & Akses</th>
                    <th className="text-left py-4 px-6 text-xs font-bold text-gray-500 uppercase tracking-wider">Status</th>
                    <th className="text-right py-4 px-6 text-xs font-bold text-gray-500 uppercase tracking-wider">Aksi</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-50">
                  {filteredStaff.map((staff) => (
                    <tr key={staff.id} className="hover:bg-gray-50/50 transition-colors group">
                      <td className="py-4 px-6">
                        <div className="flex items-center gap-3">
                          <div className="w-10 h-10 rounded-full bg-primary/10 flex items-center justify-center text-primary font-bold border border-primary/20 shrink-0">
                            {staff.name.charAt(0).toUpperCase()}
                          </div>
                          <div className="min-w-0">
                            <div className="font-bold text-gray-900 truncate">{staff.name}</div>
                            <div className="text-sm text-gray-500 truncate">{staff.email}</div>
                          </div>
                        </div>
                      </td>
                      <td className="py-4 px-6">
                        <div className="flex flex-col gap-1.5">
                          <div className="inline-flex w-fit px-2.5 py-1 rounded-md text-[11px] font-bold bg-blue-50 text-blue-700 border border-blue-100">
                            {getRoleLabel(staff.role)}
                          </div>
                          <div className="text-xs text-gray-500 flex items-center gap-1.5 line-clamp-1" title={`${staff.permissions.length} Hak Akses`}>
                            <Shield className="w-3.5 h-3.5" />
                            {staff.permissions.length} Hak Akses
                          </div>
                        </div>
                      </td>
                      <td className="py-4 px-6">
                        {staff.status === 'ACTIVE' ? (
                          <div className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-[11px] font-bold bg-green-50 text-green-700 border border-green-100">
                            <div className="w-1.5 h-1.5 rounded-full bg-green-500"></div>
                            Aktif
                          </div>
                        ) : (
                          <div className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-[11px] font-bold bg-gray-100 text-gray-600 border border-gray-200">
                            <div className="w-1.5 h-1.5 rounded-full bg-gray-400"></div>
                            Nonaktif
                          </div>
                        )}
                      </td>
                      <td className="py-4 px-6">
                        <div className="flex items-center justify-end gap-2 opacity-0 group-hover:opacity-100 transition-opacity">
                          <button
                            onClick={() => openEditModal(staff)}
                            className="p-2 text-gray-400 hover:text-blue-600 hover:bg-blue-50 rounded-lg transition-colors"
                            title="Edit Staff"
                          >
                            <Edit2 className="w-4 h-4" />
                          </button>
                          <button
                            onClick={() => toggleStatus(staff)}
                            className={`p-2 rounded-lg transition-colors ${
                              staff.status === 'ACTIVE' 
                                ? 'text-gray-400 hover:text-orange-600 hover:bg-orange-50' 
                                : 'text-gray-400 hover:text-green-600 hover:bg-green-50'
                            }`}
                            title={staff.status === 'ACTIVE' ? 'Nonaktifkan' : 'Aktifkan'}
                          >
                            <Power className="w-4 h-4" />
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </div>
      </div>

      <StaffModal
        isOpen={isModalOpen}
        onClose={() => setIsModalOpen(false)}
        onSubmit={handleModalSubmit}
        initialData={editingStaff}
        venues={venues}
      />
    </PageShell>
  );
};
