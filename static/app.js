// ===== Auth State =====
const AUTH_TOKEN_KEY = 'portal_auth_token';

function getToken() {
    return localStorage.getItem(AUTH_TOKEN_KEY);
}

function setToken(token) {
    localStorage.setItem(AUTH_TOKEN_KEY, token);
}

function clearToken() {
    localStorage.removeItem(AUTH_TOKEN_KEY);
}

function isAuthenticated() {
    return !!getToken();
}

// ===== API Client =====
const API = '/api/v1';

async function api(path, options = {}) {
    const token = getToken();
    const authHeaders = token ? { 'Authorization': 'Bearer ' + token } : {};
    const res = await fetch(API + path, {
        headers: { 'Content-Type': 'application/json', ...authHeaders, ...options.headers },
        ...options,
    });
    if (res.status === 401) {
        clearToken();
        showAuthOverlay();
        throw new Error('Session expired. Please sign in again.');
    }
    const json = await res.json();
    if (json.error) throw new Error(json.error);
    return json.data;
}

// ===== HTML Escape Helper =====
function esc(str) {
    return String(str == null ? '' : str)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;');
}

// ===== Money Helpers =====
function formatMoney(paise) {
    return '₹' + (paise / 100).toLocaleString('en-IN', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
}
function toRupees(paise) {
    return (paise / 100).toFixed(2);
}
function formatDate(dateStr) {
    if (!dateStr) return '—';
    return dateStr.slice(0, 10);
}

function confidenceBadgeHtml(c) {
    const pct = Math.round(c * 100);
    const cls = c >= 0.7 ? 'badge-paid' : (c >= 0.4 ? 'badge-partial' : 'badge-cancelled');
    return `<span class="badge ${cls}">${pct}%</span>`;
}

// ===== Routing =====
function getSection() {
    const hash = location.hash.slice(1) || 'dashboard';
    const [section, query] = hash.split('?');
    const params = new URLSearchParams(query || '');
    return { section, params };
}

function navigate(section, params = null) {
    let hash = section;
    if (params) {
        const p = new URLSearchParams(params);
        hash += '?' + p.toString();
    }
    location.hash = hash;
}

window.addEventListener('hashchange', () => {
    const { section, params } = getSection();
    renderSection(section, params);
});

document.querySelectorAll('.nav-link').forEach(link => {
    link.addEventListener('click', (e) => {
        e.preventDefault();
        navigate(link.dataset.section);
    });
});

function setActiveNav(section) {
    document.querySelectorAll('.nav-link').forEach(l => l.classList.remove('active'));
    const active = document.querySelector(`.nav-link[data-section="${section}"]`);
    if (active) active.classList.add('active');
}

// ===== Modal =====
function openModal(title, bodyHtml) {
    document.getElementById('modal-title').textContent = title;
    document.getElementById('modal-body').innerHTML = bodyHtml;
    document.getElementById('modal-overlay').classList.remove('hidden');
}

function closeModal() {
    document.getElementById('modal-overlay').classList.add('hidden');
}

document.getElementById('modal-overlay').addEventListener('click', (e) => {
    if (e.target === e.currentTarget) closeModal();
});

// ===== Section Renderer =====
const el = () => document.getElementById('section-content');

async function renderSection(section, params) {
    setActiveNav(section);
    try {
        switch (section) {
            case 'dashboard': await renderDashboard(); break;
            case 'bills': await renderBills(params); break;
            case 'invoices': await renderInvoices(params); break;
            case 'payouts': await renderPayouts(params); break;
            case 'transactions': await renderTransactions(params); break;
            case 'accounts': await renderAccounts(params); break;
            case 'contacts': await renderContacts(params); break;
            case 'recurring-payments': await renderRecurringPayments(params); break;
            default: el().innerHTML = '<div class="empty-state"><p>Section not found</p></div>';
        }
    } catch (err) {
        el().innerHTML = `<div class="empty-state"><p>Error: ${err.message}</p></div>`;
    }
}

// ===== Dashboard =====
async function renderDashboard() {
    const d = await api('/dashboard');
    el().innerHTML = `
        <div class="section-header"><h1>Dashboard</h1></div>
        <div class="cards-grid">
            <div class="card stat-card">
                <div class="stat-label">Bills Payable</div>
                <div class="stat-value money money-expense">${formatMoney(d.bills_payable)}</div>
                <div class="stat-sub">${d.total_bills} total · ${d.overdue_bills} overdue</div>
            </div>
            <div class="card stat-card">
                <div class="stat-label">Invoices Receivable</div>
                <div class="stat-value money money-income">${formatMoney(d.invoices_receivable)}</div>
                <div class="stat-sub">${d.total_invoices} total · ${d.overdue_invoices} overdue</div>
            </div>
            <div class="card stat-card">
                <div class="stat-label">Payouts Received</div>
                <div class="stat-value money money-income">${formatMoney(d.payouts_received)}</div>
                <div class="stat-sub">${d.total_payouts} total payouts</div>
            </div>
            <div class="card stat-card">
                <div class="stat-label">Accounts</div>
                <div class="stat-value">${d.total_accounts}</div>
            </div>
            <div class="card stat-card">
                <div class="stat-label">Contacts</div>
                <div class="stat-value">${d.total_contacts}</div>
            </div>
        </div>
        <h2 style="font-size:1.1rem;margin-bottom:1rem;">Recent Transactions</h2>
        <div class="table-wrap">
            <table>
                <thead><tr><th>Date</th><th>Type</th><th>Account</th><th>Description</th><th>Amount</th></tr></thead>
                <tbody>
                    ${d.recent_transactions.length === 0 ? '<tr><td colspan="5" class="empty-state">No transactions yet</td></tr>' :
            d.recent_transactions.map(t => `<tr>
                        <td>${formatDate(t.transaction_date)}</td>
                        <td><span class="badge badge-${t.type}">${t.type}</span></td>
                        <td>${t.account_name || '—'}</td>
                        <td>${t.description || '—'}</td>
                        <td class="money ${t.type === 'income' ? 'money-income' : 'money-expense'}">${formatMoney(t.amount)}</td>
                    </tr>`).join('')}
                </tbody>
            </table>
        </div>
    `;
}

// ===== Accounts =====
async function renderAccounts(params) {
    const search = params?.get('search') || '';
    const accounts = await api('/accounts' + (search ? '?search=' + encodeURIComponent(search) : ''));
    el().innerHTML = `
        <div class="section-header">
            <h1>Accounts</h1>
            <div style="display:flex;gap:0.5rem">
                <input type="search" class="form-control" placeholder="Search name..." value="${search}" onkeypress="if(event.key==='Enter') navigate('accounts', {search:this.value})">
                <button class="btn btn-primary" onclick="showAccountForm()">+ New Account</button>
            </div>
        </div>
        <div class="table-wrap">
            <table>
                <thead><tr><th>Name</th><th>Type</th><th>Opening</th><th>Current Balance</th><th>Actions</th></tr></thead>
                <tbody>
                    ${accounts.length === 0 ? '<tr><td colspan="5" class="empty-state">No accounts found</td></tr>' :
            accounts.map(a => `<tr>
                        <td>${a.name}</td>
                        <td><span class="badge badge-${a.type}">${a.type.replace('_', ' ')}</span></td>
                        <td class="money">${formatMoney(a.opening_balance)}</td>
                        <td>
                            <strong class="money">${formatMoney(a.balance)}</strong>
                            <br><button class="btn-link" onclick="navigate('transactions', {account_id: ${a.id}})">View Txns</button>
                        </td>
                        <td class="actions-cell">
                            <button class="btn btn-ghost btn-sm" onclick="showAccountForm(${a.id})">Edit</button>
                            <button class="btn btn-danger btn-sm" onclick="deleteAccount(${a.id})">Delete</button>
                        </td>
                    </tr>`).join('')}
                </tbody>
            </table>
        </div>
    `;
}

async function showAccountForm(id) {
    let data = { name: '', type: 'bank', opening_balance: 0 };
    if (id) {
        data = await api(`/accounts/${id}`);
    }
    openModal(id ? 'Edit Account' : 'New Account', `
        <form onsubmit="saveAccount(event, ${id || 'null'})">
            <div class="form-group">
                <label>Name</label>
                <input class="form-control" name="name" value="${data.name}" required>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>Type</label>
                    <select class="form-control" name="type">
                        <option value="bank" ${data.type === 'bank' ? 'selected' : ''}>Bank</option>
                        <option value="cash" ${data.type === 'cash' ? 'selected' : ''}>Cash</option>
                        <option value="credit_card" ${data.type === 'credit_card' ? 'selected' : ''}>Credit Card</option>
                    </select>
                </div>
                <div class="form-group">
                    <label>Opening Balance (₹)</label>
                    <input class="form-control" name="opening_balance" type="number" step="0.01" value="${toRupees(data.opening_balance)}">
                </div>
            </div>
            <div class="form-actions">
                <button type="button" class="btn btn-ghost" onclick="closeModal()">Cancel</button>
                <button type="submit" class="btn btn-primary">${id ? 'Update' : 'Create'}</button>
            </div>
        </form>
    `);
}

async function saveAccount(e, id) {
    e.preventDefault();
    const form = e.target;
    const body = JSON.stringify({
        name: form.name.value,
        type: form.type.value,
        opening_balance: parseFloat(form.opening_balance.value || 0),
    });
    if (id) {
        await api(`/accounts/${id}`, { method: 'PUT', body });
    } else {
        await api('/accounts', { method: 'POST', body });
    }
    closeModal();
    renderAccounts();
}

async function deleteAccount(id) {
    if (!confirm('Delete this account?')) return;
    await api(`/accounts/${id}`, { method: 'DELETE' });
    renderAccounts();
}

// ===== Contacts =====
async function renderContacts(params) {
    const search = params?.get('search') || '';
    const contacts = await api('/contacts' + (search ? '?search=' + encodeURIComponent(search) : ''));
    el().innerHTML = `
        <div class="section-header">
            <h1>Contacts</h1>
            <div style="display:flex;gap:0.5rem">
                <input type="search" class="form-control" placeholder="Search name/email/phone..." value="${search}" onkeypress="if(event.key==='Enter') navigate('contacts', {search:this.value})">
                <button class="btn btn-primary" onclick="showContactForm()">+ New Contact</button>
            </div>
        </div>
        <div class="table-wrap">
            <table>
                <thead><tr><th>Name</th><th>Type</th><th>Total</th><th>Paid/Recv</th><th>Balance</th><th>Actions</th></tr></thead>
                <tbody>
                    ${contacts.length === 0 ? '<tr><td colspan="6" class="empty-state">No contacts found</td></tr>' :
            contacts.map(c => `<tr>
                        <td>
                            <strong>${c.name}</strong><br>
                            <small style="color:var(--text-muted)">${c.email || ''}</small>
                        </td>
                        <td><span class="badge badge-${c.type}">${c.type}</span></td>
                        <td class="money">${formatMoney(c.total_amount)}</td>
                        <td class="money">${formatMoney(c.allocated_amount)}</td>
                        <td>
                            <strong class="money">${formatMoney(c.balance)}</strong>
                            <div style="margin-top:0.25rem;display:flex;gap:0.5rem">
                                <button class="btn-link" onclick="navigate('bills', {contact_id: ${c.id}})">Bills</button>
                                <button class="btn-link" onclick="navigate('invoices', {contact_id: ${c.id}})">Invoices</button>
                                <button class="btn-link" onclick="navigate('transactions', {contact_id: ${c.id}})">Txns</button>
                            </div>
                        </td>
                        <td class="actions-cell">
                            <button class="btn btn-ghost btn-sm" onclick="showContactForm(${c.id})">Edit</button>
                            <button class="btn btn-danger btn-sm" onclick="deleteContact(${c.id})">Delete</button>
                        </td>
                    </tr>`).join('')}
                </tbody>
            </table>
        </div>
    `;
}

async function showContactForm(id) {
    let data = { name: '', type: 'vendor', email: '', phone: '' };
    if (id) data = await api(`/contacts/${id}`);
    openModal(id ? 'Edit Contact' : 'New Contact', `
        <form onsubmit="saveContact(event, ${id || 'null'})">
            <div class="form-group">
                <label>Name</label>
                <input class="form-control" name="name" value="${data.name}" required>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>Type</label>
                    <select class="form-control" name="type">
                        <option value="vendor" ${data.type === 'vendor' ? 'selected' : ''}>Vendor</option>
                        <option value="customer" ${data.type === 'customer' ? 'selected' : ''}>Customer</option>
                    </select>
                </div>
                <div class="form-group">
                    <label>Email</label>
                    <input class="form-control" name="email" type="email" value="${data.email || ''}">
                </div>
            </div>
            <div class="form-group">
                <label>Phone</label>
                <input class="form-control" name="phone" value="${data.phone || ''}">
            </div>
            <div class="form-actions">
                <button type="button" class="btn btn-ghost" onclick="closeModal()">Cancel</button>
                <button type="submit" class="btn btn-primary">${id ? 'Update' : 'Create'}</button>
            </div>
        </form>
    `);
}

async function saveContact(e, id) {
    e.preventDefault();
    const f = e.target;
    const body = JSON.stringify({
        name: f.name.value,
        type: f.type.value,
        email: f.email.value || null,
        phone: f.phone.value || null,
    });
    if (id) await api(`/contacts/${id}`, { method: 'PUT', body });
    else await api('/contacts', { method: 'POST', body });
    closeModal();
    renderContacts();
}

async function deleteContact(id) {
    if (!confirm('Delete this contact?')) return;
    await api(`/contacts/${id}`, { method: 'DELETE' });
    renderContacts();
}

// ===== Bills =====
async function renderBills(params) {
    const cid = params?.get('contact_id');
    const search = params?.get('search') || '';
    let url = '/bills?';
    if (cid) url += 'contact_id=' + cid + '&';
    if (search) url += 'search=' + encodeURIComponent(search);

    const [bills, contacts] = await Promise.all([
        api(url),
        api('/contacts?type=vendor')
    ]);
    window._contacts = contacts;
    el().innerHTML = `
        <div class="section-header">
            <h1>Bills ${cid ? ' - Vendor Filter' : ''}</h1>
            <div style="display:flex;gap:0.5rem">
                <input type="search" class="form-control" placeholder="Search bill #, notes, vendor..." value="${search}" onkeypress="if(event.key==='Enter') navigate('bills', {contact_id:'${cid || ''}', search:this.value})">
                <button class="btn btn-primary" onclick="showBillForm()">+ New Bill</button>
            </div>
        </div>
        <div class="table-wrap">
            <table>
                <thead><tr><th>Number</th><th>Contact</th><th>Due Date</th><th>Amount</th><th>Allocated</th><th>Status</th><th>Payments</th><th>Actions</th></tr></thead>
                <tbody>
                    ${bills.length === 0 ? '<tr><td colspan="8" class="empty-state">No bills found</td></tr>' :
            bills.map(b => `<tr>
                        <td>${b.bill_number || '—'}</td>
                        <td>${b.contact_name || '—'}</td>
                        <td>${formatDate(b.due_date)}</td>
                        <td class="money">${formatMoney(b.amount)}</td>
                        <td>
                            <span class="money">${formatMoney(b.allocated)}</span>
                            ${b.amount > 0 ? `<div class="alloc-bar"><div class="alloc-bar-fill ${b.allocated >= b.amount ? 'full' : ''}" style="width:${Math.min(100, (b.allocated / b.amount) * 100)}%"></div></div>` : ''}
                        </td>
                        <td><span class="badge badge-${b.status}">${b.status}</span></td>
                        <td><button class="btn-link" onclick="showDocumentLinks('bill', ${b.id})">View Links</button></td>
                        <td class="actions-cell">
                            <button class="btn btn-ghost btn-sm" onclick="showBillForm(${b.id})">Edit</button>
                            <button class="btn btn-danger btn-sm" onclick="deleteBill(${b.id})">Delete</button>
                        </td>
                    </tr>`).join('')}
                </tbody>
            </table>
        </div>
    `;
}

async function showDocumentLinks(type, id) {
    const [links, suggestions, docData] = await Promise.all([
        api(`/${type}s/${id}/links`),
        api(`/${type}s/${id}/match-suggestions`),
        api(`/${type}s/${id}`),
    ]);
    let title = 'Document';
    if (type === 'bill') title = 'Bill';
    else if (type === 'invoice') title = 'Invoice';
    else if (type === 'payout') title = 'Payout';
    const docUnallocated = docData.unallocated || 0;
    openModal(`${title} Payments (#${id})`, `
        <table style="margin-bottom:1rem">
            <thead><tr><th>Date</th><th>Ref</th><th>Account</th><th>Amount</th></tr></thead>
            <tbody>
                ${links.length === 0 ? '<tr><td colspan="4" class="empty-state">No payments linked</td></tr>' :
            links.map(l => `<tr>
                    <td>${formatDate(l.transaction_date)}</td>
                    <td>${l.reference || '—'}</td>
                    <td>${l.account_name}</td>
                    <td class="money">${formatMoney(l.amount)}</td>
                </tr>`).join('')}
            </tbody>
        </table>

        ${docUnallocated > 0 ? `
        <h3 style="font-size:0.9rem;margin-bottom:0.5rem">Suggested Transactions
            <span style="color:var(--text-muted);font-weight:400"> — ${formatMoney(docUnallocated)} unallocated</span>
        </h3>
        ${suggestions.length === 0 ? '<p style="color:var(--text-muted);margin-bottom:1rem;font-size:0.85rem">No matching transactions found.</p>' : `
        <table style="margin-bottom:1rem">
            <thead><tr><th>Date</th><th>Description</th><th>Unallocated</th><th>Confidence</th><th></th></tr></thead>
            <tbody>
                ${suggestions.map(s => {
                    const linkAmt = Math.min(s.unallocated, docUnallocated);
                    return `<tr>
                        <td>${formatDate(s.transaction_date)}</td>
                        <td>${esc(s.description || s.reference || '—')}</td>
                        <td class="money">${formatMoney(s.unallocated)}</td>
                        <td>${confidenceBadgeHtml(s.confidence)}</td>
                        <td>${s.linkable ? `<button class="btn btn-primary btn-sm" onclick="linkDocumentToTransaction('${type}', ${id}, ${s.transaction_id}, ${linkAmt})">Link</button>` : ''}</td>
                    </tr>`;
                }).join('')}
            </tbody>
        </table>`}
        ` : ''}

        <div class="form-actions">
            <button class="btn btn-primary" onclick="closeModal()">Close</button>
        </div>
    `);
}

async function postTransactionLink(txnId, docType, docId, amountPaise, onSuccess) {
    try {
        await api(`/transactions/${txnId}/links`, {
            method: 'POST',
            body: JSON.stringify({
                document_type: docType,
                document_id: docId,
                amount: amountPaise / 100,
            }),
        });
        onSuccess();
    } catch (err) {
        alert('Error: ' + err.message);
    }
}

async function linkDocumentToTransaction(docType, docId, txnId, amountPaise) {
    await postTransactionLink(txnId, docType, docId, amountPaise, () => showDocumentLinks(docType, docId));
}

async function showBillForm(id) {
    let data = { bill_number: '', contact_id: null, issue_date: '', due_date: '', amount: 0, status: 'draft', file_url: '', notes: '' };
    if (id) data = await api(`/bills/${id}`);
    const contacts = window._contacts || await api('/contacts?type=vendor');
    openModal(id ? 'Edit Bill' : 'New Bill', `
        <form onsubmit="saveBill(event, ${id || 'null'})">
            <div class="form-row">
                <div class="form-group">
                    <label>Bill Number</label>
                    <input class="form-control" name="bill_number" value="${data.bill_number || ''}">
                </div>
                <div class="form-group">
                    <label>Contact (optional)</label>
                    <select class="form-control" name="contact_id">
                        <option value="">— None —</option>
                        ${contacts.map(c => `<option value="${c.id}" ${data.contact_id == c.id ? 'selected' : ''}>${c.name}</option>`).join('')}
                    </select>
                </div>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>Issue Date</label>
                    <input class="form-control" name="issue_date" type="date" value="${data.issue_date || ''}">
                </div>
                <div class="form-group">
                    <label>Due Date</label>
                    <input class="form-control" name="due_date" type="date" value="${data.due_date || ''}">
                </div>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>Amount (₹)</label>
                    <input class="form-control" name="amount" type="number" step="0.01" value="${toRupees(data.amount)}" required>
                </div>
                <div class="form-group">
                    <label>Status</label>
                    <select class="form-control" name="status">
                        ${['draft', 'received', 'paid', 'overdue', 'cancelled'].map(s => `<option value="${s}" ${data.status === s ? 'selected' : ''}>${s}</option>`).join('')}
                    </select>
                </div>
            </div>
            <div class="form-group">
                <label>File URL (optional)</label>
                <input class="form-control" name="file_url" value="${data.file_url || ''}">
            </div>
            <div class="form-group">
                <label>Notes</label>
                <textarea class="form-control" name="notes">${data.notes || ''}</textarea>
            </div>
            <div class="form-actions">
                <button type="button" class="btn btn-ghost" onclick="closeModal()">Cancel</button>
                <button type="submit" class="btn btn-primary">${id ? 'Update' : 'Create'}</button>
            </div>
        </form>
    `);
}

async function saveBill(e, id) {
    e.preventDefault();
    const f = e.target;
    const body = JSON.stringify({
        bill_number: f.bill_number.value,
        contact_id: f.contact_id.value ? parseInt(f.contact_id.value) : null,
        issue_date: f.issue_date.value || null,
        due_date: f.due_date.value || null,
        amount: parseFloat(f.amount.value || 0),
        status: f.status.value,
        file_url: f.file_url.value || null,
        notes: f.notes.value || null,
    });
    if (id) await api(`/bills/${id}`, { method: 'PUT', body });
    else await api('/bills', { method: 'POST', body });
    closeModal();
    renderBills();
}

async function deleteBill(id) {
    if (!confirm('Delete this bill?')) return;
    await api(`/bills/${id}`, { method: 'DELETE' });
    renderBills();
}

// ===== Invoices =====
async function renderInvoices(params) {
    const cid = params?.get('contact_id');
    const search = params?.get('search') || '';
    let url = '/invoices?';
    if (cid) url += 'contact_id=' + cid + '&';
    if (search) url += 'search=' + encodeURIComponent(search);

    const [invoices, contacts] = await Promise.all([
        api(url),
        api('/contacts?type=customer')
    ]);
    window._customerContacts = contacts;
    el().innerHTML = `
        <div class="section-header">
            <h1>Invoices ${cid ? ' - Customer Filter' : ''}</h1>
            <div style="display:flex;gap:0.5rem">
                <input type="search" class="form-control" placeholder="Search invoice #, notes, customer..." value="${search}" onkeypress="if(event.key==='Enter') navigate('invoices', {contact_id:'${cid || ''}', search:this.value})">
                <button class="btn btn-primary" onclick="showInvoiceForm()">+ New Invoice</button>
            </div>
        </div>
        <div class="table-wrap">
            <table>
                <thead><tr><th>Number</th><th>Contact</th><th>Due Date</th><th>Amount</th><th>Received</th><th>Status</th><th>Payments</th><th>Actions</th></tr></thead>
                <tbody>
                    ${invoices.length === 0 ? '<tr><td colspan="9" class="empty-state">No invoices found</td></tr>' :
            invoices.map(inv => `<tr>
                        <td>${inv.invoice_number || '—'}</td>
                        <td>${inv.contact_name || '—'}</td>
                        <td>${formatDate(inv.due_date)}</td>
                        <td class="money">${formatMoney(inv.amount)}</td>
                        <td>
                            <span class="money">${formatMoney(inv.allocated)}</span>
                            ${inv.amount > 0 ? `<div class="alloc-bar"><div class="alloc-bar-fill ${inv.allocated >= inv.amount ? 'full' : ''}" style="width:${Math.min(100, (inv.allocated / inv.amount) * 100)}%"></div></div>` : ''}
                        </td>
                        <td><span class="badge badge-${inv.status}">${inv.status}</span></td>
                        <td><button class="btn-link" onclick="showDocumentLinks('invoice', ${inv.id})">View Links</button></td>
                        <td class="actions-cell">
                            <button class="btn btn-ghost btn-sm" onclick="showInvoiceForm(${inv.id})">Edit</button>
                            <button class="btn btn-danger btn-sm" onclick="deleteInvoice(${inv.id})">Delete</button>
                        </td>
                    </tr>`).join('')}
                </tbody>
            </table>
        </div>
    `;
}

async function showInvoiceForm(id) {
    let data = { invoice_number: '', contact_id: null, issue_date: '', due_date: '', amount: 0, status: 'draft', file_url: '', notes: '' };
    if (id) data = await api(`/invoices/${id}`);
    const contacts = window._customerContacts || await api('/contacts?type=customer');
    openModal(id ? 'Edit Invoice' : 'New Invoice', `
        <form onsubmit="saveInvoice(event, ${id || 'null'})">
            <div class="form-row">
                <div class="form-group">
                    <label>Invoice Number</label>
                    <input class="form-control" name="invoice_number" value="${data.invoice_number || ''}">
                </div>
                <div class="form-group">
                    <label>Contact (optional)</label>
                    <select class="form-control" name="contact_id">
                        <option value="">— None —</option>
                        ${contacts.map(c => `<option value="${c.id}" ${data.contact_id == c.id ? 'selected' : ''}>${c.name}</option>`).join('')}
                    </select>
                </div>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>Issue Date</label>
                    <input class="form-control" name="issue_date" type="date" value="${data.issue_date || ''}">
                </div>
                <div class="form-group">
                    <label>Due Date</label>
                    <input class="form-control" name="due_date" type="date" value="${data.due_date || ''}">
                </div>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>Amount (₹)</label>
                    <input class="form-control" name="amount" type="number" step="0.01" value="${toRupees(data.amount)}" required>
                </div>
                <div class="form-group">
                    <label>Status</label>
                    <select class="form-control" name="status">
                        ${['draft', 'sent', 'paid', 'received', 'overdue', 'cancelled'].map(s => `<option value="${s}" ${data.status === s ? 'selected' : ''}>${s}</option>`).join('')}
                    </select>
                </div>
            </div>
            <div class="form-group">
                <label>File URL (optional)</label>
                <input class="form-control" name="file_url" value="${data.file_url || ''}">
            </div>
            <div class="form-group">
                <label>Notes</label>
                <textarea class="form-control" name="notes">${data.notes || ''}</textarea>
            </div>
            <div class="form-actions">
                <button type="button" class="btn btn-ghost" onclick="closeModal()">Cancel</button>
                <button type="submit" class="btn btn-primary">${id ? 'Update' : 'Create'}</button>
            </div>
        </form>
    `);
}

async function saveInvoice(e, id) {
    e.preventDefault();
    const f = e.target;
    const body = JSON.stringify({
        invoice_number: f.invoice_number.value,
        contact_id: f.contact_id.value ? parseInt(f.contact_id.value) : null,
        issue_date: f.issue_date.value || null,
        due_date: f.due_date.value || null,
        amount: parseFloat(f.amount.value || 0),
        status: f.status.value,
        file_url: f.file_url.value || null,
        notes: f.notes.value || null,
    });
    if (id) await api(`/invoices/${id}`, { method: 'PUT', body });
    else await api('/invoices', { method: 'POST', body });
    closeModal();
    renderInvoices();
}

async function deleteInvoice(id) {
    if (!confirm('Delete this invoice?')) return;
    await api(`/invoices/${id}`, { method: 'DELETE' });
    renderInvoices();
}

// ===== Payouts =====
async function renderPayouts(params) {
    const platform = params?.get('platform') || '';
    const outlet = params?.get('outlet_name') || '';
    let url = '/payouts?';
    if (platform) url += 'platform=' + platform + '&';
    if (outlet) url += 'outlet_name=' + encodeURIComponent(outlet);

    const payouts = await api(url);
    el().innerHTML = `
        <div class="section-header">
            <h1>Platform Payouts</h1>
            <div style="display:flex;gap:0.5rem">
                <select class="form-control" onchange="navigate('payouts', {platform:this.value, outlet_name:'${outlet}'})">
                    <option value="">— All Platforms —</option>
                    <option value="Swiggy" ${platform === 'Swiggy' ? 'selected' : ''}>Swiggy</option>
                    <option value="Zomato" ${platform === 'Zomato' ? 'selected' : ''}>Zomato</option>
                </select>
                <input type="search" class="form-control" placeholder="Search outlet..." value="${outlet}" onkeypress="if(event.key==='Enter') navigate('payouts', {platform:'${platform}', outlet_name:this.value})">
                <button class="btn btn-primary" onclick="showPayoutForm()">+ New Payout</button>
            </div>
        </div>
        <div class="table-wrap">
            <table>
                <thead><tr><th>Date</th><th>Platform</th><th>Outlet</th><th>Orders</th><th>Gross Sales</th><th>Final Payout</th><th>Allocated</th><th>UTR</th><th>Payments</th><th>Actions</th></tr></thead>
                <tbody>
                    ${payouts.length === 0 ? '<tr><td colspan="10" class="empty-state">No payouts found</td></tr>' :
            payouts.map(p => `<tr>
                        <td>${esc(formatDate(p.settlement_date))}</td>
                        <td><span class="badge badge-${p.platform}">${p.platform}</span></td>
                        <td>${p.outlet_name}</td>
                        <td>${p.total_orders}</td>
                        <td class="money">${formatMoney(p.gross_sales_amt)}</td>
                        <td class="money money-income">${formatMoney(p.final_payout_amt)}</td>
                        <td>
                            <span class="money">${formatMoney(p.allocated)}</span>
                            ${p.final_payout_amt > 0 ? `<div class="alloc-bar"><div class="alloc-bar-fill ${p.allocated >= p.final_payout_amt ? 'full' : ''}" style="width:${Math.min(100, (p.allocated / p.final_payout_amt) * 100)}%"></div></div>` : ''}
                        </td>
                        <td><small>${p.utr_number || '—'}</small></td>
                        <td><button class="btn-link" onclick="showDocumentLinks('payout', ${p.id})">View Links</button></td>
                        <td class="actions-cell">
                            <button class="btn btn-ghost btn-sm" onclick="showPayoutForm(${p.id})">Edit</button>
                            <button class="btn btn-danger btn-sm" onclick="deletePayout(${p.id})">Delete</button>
                        </td>
                    </tr>`).join('')}
                </tbody>
            </table>
        </div>
    `;
}

async function showPayoutForm(id) {
    let data = {
        outlet_name: '', platform: 'Swiggy', period_start: '', period_end: '', settlement_date: '',
        total_orders: 0, gross_sales_amt: 0, restaurant_discount_amt: 0, platform_commission_amt: 0,
        taxes_tcs_tds_amt: 0, marketing_ads_amt: 0, final_payout_amt: 0, utr_number: ''
    };
    if (id) data = await api(`/payouts/${id}`);
    openModal(id ? 'Edit Payout' : 'New Payout', `
        <form onsubmit="savePayout(event, ${id || 'null'})">
            <div class="form-row">
                <div class="form-group">
                    <label>Outlet Name</label>
                    <input class="form-control" name="outlet_name" value="${data.outlet_name}" required>
                </div>
                <div class="form-group">
                    <label>Platform</label>
                    <select class="form-control" name="platform">
                        <option value="Swiggy" ${data.platform === 'Swiggy' ? 'selected' : ''}>Swiggy</option>
                        <option value="Zomato" ${data.platform === 'Zomato' ? 'selected' : ''}>Zomato</option>
                    </select>
                </div>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>Period Start</label>
                    <input class="form-control" name="period_start" type="date" value="${data.period_start || ''}">
                </div>
                <div class="form-group">
                    <label>Period End</label>
                    <input class="form-control" name="period_end" type="date" value="${data.period_end || ''}">
                </div>
                <div class="form-group">
                    <label>Settlement Date</label>
                    <input class="form-control" name="settlement_date" type="date" value="${data.settlement_date || ''}">
                </div>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>Total Orders</label>
                    <input class="form-control" name="total_orders" type="number" value="${data.total_orders}">
                </div>
                <div class="form-group">
                    <label>UTR Number</label>
                    <input class="form-control" name="utr_number" value="${data.utr_number || ''}">
                </div>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>Gross Sales (₹)</label>
                    <input class="form-control" name="gross_sales_amt" type="number" step="0.01" value="${toRupees(data.gross_sales_amt)}" required>
                </div>
                <div class="form-group">
                    <label>Restaurant Discount (₹)</label>
                    <input class="form-control" name="restaurant_discount_amt" type="number" step="0.01" value="${toRupees(data.restaurant_discount_amt)}">
                </div>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>Commission (₹)</label>
                    <input class="form-control" name="platform_commission_amt" type="number" step="0.01" value="${toRupees(data.platform_commission_amt)}">
                </div>
                <div class="form-group">
                    <label>Taxes/TCS/TDS (₹)</label>
                    <input class="form-control" name="taxes_tcs_tds_amt" type="number" step="0.01" value="${toRupees(data.taxes_tcs_tds_amt)}">
                </div>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>Marketing/Ads (₹)</label>
                    <input class="form-control" name="marketing_ads_amt" type="number" step="0.01" value="${toRupees(data.marketing_ads_amt)}">
                </div>
                <div class="form-group">
                    <label>Final Payout (₹)</label>
                    <input class="form-control" name="final_payout_amt" type="number" step="0.01" value="${toRupees(data.final_payout_amt)}" required>
                </div>
            </div>
            <div class="form-actions">
                <button type="button" class="btn btn-ghost" onclick="closeModal()">Cancel</button>
                <button type="submit" class="btn btn-primary">${id ? 'Update' : 'Create'}</button>
            </div>
        </form>
    `);
}

async function savePayout(e, id) {
    e.preventDefault();
    const f = e.target;
    const body = JSON.stringify({
        outlet_name: f.outlet_name.value,
        platform: f.platform.value,
        period_start: f.period_start.value || null,
        period_end: f.period_end.value || null,
        settlement_date: f.settlement_date.value || null,
        total_orders: parseInt(f.total_orders.value || 0),
        gross_sales_amt: parseFloat(f.gross_sales_amt.value || 0),
        restaurant_discount_amt: parseFloat(f.restaurant_discount_amt.value || 0),
        platform_commission_amt: parseFloat(f.platform_commission_amt.value || 0),
        taxes_tcs_tds_amt: parseFloat(f.taxes_tcs_tds_amt.value || 0),
        marketing_ads_amt: parseFloat(f.marketing_ads_amt.value || 0),
        final_payout_amt: parseFloat(f.final_payout_amt.value || 0),
        utr_number: f.utr_number.value || '',
    });
    if (id) await api(`/payouts/${id}`, { method: 'PUT', body });
    else await api('/payouts', { method: 'POST', body });
    closeModal();
    renderPayouts();
}

async function deletePayout(id) {
    if (!confirm('Delete this payout record?')) return;
    await api(`/payouts/${id}`, { method: 'DELETE' });
    renderPayouts();
}

// ===== Transactions =====
async function renderTransactions(params) {
    let url = '/transactions';
    const aid = params?.get('account_id');
    const cid = params?.get('contact_id');
    if (aid) url += '?account_id=' + aid;
    else if (cid) url += '?contact_id=' + cid;

    const [txns, accounts, contacts] = await Promise.all([
        api(url), api('/accounts'), api('/contacts')
    ]);
    window._accounts = accounts;
    window._allContacts = contacts;

    el().innerHTML = `
        <div class="section-header">
            <h1>Transactions ${aid ? ' - Account Filter' : (cid ? ' - Contact Filter' : '')}</h1>
            <div>
                ${(aid || cid) ? '<button class="btn btn-ghost" onclick="navigate(\'transactions\')" style="margin-right:0.5rem">View All</button>' : ''}
                <button class="btn btn-primary" onclick="showTransactionForm()">+ New Transaction</button>
            </div>
        </div>
        <div class="table-wrap">
            <table>
                <thead><tr><th>Date</th><th>Type</th><th>Account</th><th>Description</th><th>Amount</th><th>Allocated</th><th>Links</th><th>Actions</th></tr></thead>
                <tbody>
                    ${txns.length === 0 ? '<tr><td colspan="8" class="empty-state">No transactions yet</td></tr>' :
            txns.map(t => `<tr>
                        <td>${formatDate(t.transaction_date)}</td>
                        <td><span class="badge badge-${t.type}">${t.type}</span></td>
                        <td>${t.account_name || '—'}${t.type === 'expense' && t.transfer_account_name ? ' → ' + t.transfer_account_name : ''}</td>
                        <td>${t.description || '—'}${t.contact_name ? '<br><small style="color:var(--text-muted)">' + t.contact_name + '</small>' : ''}</td>
                        <td class="money ${t.type === 'income' ? 'money-income' : 'money-expense'}">${formatMoney(t.amount)}</td>
                        <td>
                            <span class="money">${formatMoney(t.allocated)}</span>
                            ${t.amount > 0 ? `<div class="alloc-bar"><div class="alloc-bar-fill ${t.allocated >= t.amount ? 'full' : ''}" style="width:${Math.min(100, (t.allocated / t.amount) * 100)}%"></div></div>` : ''}
                        </td>
                        <td><button class="btn-link" onclick="showTransactionLinks(${t.id})">Manage</button></td>
                        <td class="actions-cell">
                            <button class="btn btn-ghost btn-sm" onclick="showTransactionForm(${t.id})">Edit</button>
                            <button class="btn btn-danger btn-sm" onclick="deleteTransaction(${t.id})">Delete</button>
                        </td>
                    </tr>`).join('')}
                </tbody>
            </table>
        </div>
    `;
}

async function showTransactionForm(id) {
    let data = { account_id: '', type: 'expense', amount: 0, transaction_date: '', description: '', reference: '', transfer_account_id: null, contact_id: null };
    if (id) data = await api(`/transactions/${id}`);
    const accounts = window._accounts || await api('/accounts');
    const contacts = window._allContacts || await api('/contacts');

    openModal(id ? 'Edit Transaction' : 'New Transaction', `
        <form onsubmit="saveTransaction(event, ${id || 'null'})">
            <div class="form-row">
                <div class="form-group">
                    <label>Type</label>
                    <select class="form-control" name="type" onchange="toggleTransferField(this.value)">
                        <option value="income" ${data.type === 'income' ? 'selected' : ''}>Income</option>
                        <option value="expense" ${data.type === 'expense' ? 'selected' : ''}>Expense</option>
                        <option value="transfer" ${data.type === 'transfer' ? 'selected' : ''}>Transfer</option>
                    </select>
                </div>
                <div class="form-group">
                    <label>Amount (₹)</label>
                    <input class="form-control" name="amount" type="number" step="0.01" value="${toRupees(data.amount)}" required>
                </div>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>Account</label>
                    <select class="form-control" name="account_id" required>
                        <option value="">Select account</option>
                        ${accounts.map(a => `<option value="${a.id}" ${data.account_id == a.id ? 'selected' : ''}>${a.name}</option>`).join('')}
                    </select>
                </div>
                <div class="form-group" id="transfer-account-group" style="display:${data.type === 'transfer' ? 'block' : 'none'}">
                    <label>Transfer To</label>
                    <select class="form-control" name="transfer_account_id">
                        <option value="">Select account</option>
                        ${accounts.map(a => `<option value="${a.id}" ${data.transfer_account_id == a.id ? 'selected' : ''}>${a.name}</option>`).join('')}
                    </select>
                </div>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>Date</label>
                    <input class="form-control" name="transaction_date" type="date" value="${data.transaction_date || ''}">
                </div>
                <div class="form-group">
                    <label>Contact (optional)</label>
                    <select class="form-control" name="contact_id">
                        <option value="">— None —</option>
                        ${contacts.map(c => `<option value="${c.id}" ${data.contact_id == c.id ? 'selected' : ''}>${c.name}</option>`).join('')}
                    </select>
                </div>
            </div>
            <div class="form-group">
                <label>Description</label>
                <input class="form-control" name="description" value="${data.description || ''}">
            </div>
            <div class="form-group">
                <label>Reference</label>
                <input class="form-control" name="reference" value="${data.reference || ''}">
            </div>
            <div class="form-actions">
                <button type="button" class="btn btn-ghost" onclick="closeModal()">Cancel</button>
                <button type="submit" class="btn btn-primary">${id ? 'Update' : 'Create'}</button>
            </div>
        </form>
    `);
}

function toggleTransferField(type) {
    document.getElementById('transfer-account-group').style.display = type === 'transfer' ? 'block' : 'none';
}

async function saveTransaction(e, id) {
    e.preventDefault();
    const f = e.target;
    const body = JSON.stringify({
        account_id: parseInt(f.account_id.value),
        type: f.type.value,
        amount: parseFloat(f.amount.value || 0),
        transaction_date: f.transaction_date.value || null,
        description: f.description.value || null,
        reference: f.reference.value || null,
        transfer_account_id: f.transfer_account_id.value ? parseInt(f.transfer_account_id.value) : null,
        contact_id: f.contact_id.value ? parseInt(f.contact_id.value) : null,
    });
    try {
        if (id) await api(`/transactions/${id}`, { method: 'PUT', body });
        else await api('/transactions', { method: 'POST', body });
        closeModal();
        renderTransactions();
    } catch (err) {
        alert('Error: ' + err.message);
    }
}

async function deleteTransaction(id) {
    if (!confirm('Delete this transaction?')) return;
    await api(`/transactions/${id}`, { method: 'DELETE' });
    renderTransactions();
}

// ===== Transaction Links (Allocation) =====
async function showTransactionLinks(txnId) {
    const [links, txn, bills, invoices, payouts, suggestions] = await Promise.all([
        api(`/transactions/${txnId}/links`),
        api(`/transactions/${txnId}`),
        api('/bills'),
        api('/invoices'),
        api('/payouts'),
        api(`/transactions/${txnId}/match-suggestions`),
    ]);

    const unallocated = txn.unallocated;

    openModal(`Links — Transaction #${txnId}`, `
        <div style="margin-bottom:1rem">
            <span class="money">${formatMoney(txn.amount)}</span> total ·
            <span class="money money-income">${formatMoney(txn.allocated)}</span> allocated ·
            <span class="money" style="color:var(--warning)">${formatMoney(unallocated)}</span> unallocated
            <div class="alloc-bar" style="margin-top:0.5rem"><div class="alloc-bar-fill ${txn.allocated >= txn.amount ? 'full' : ''}" style="width:${txn.amount > 0 ? Math.min(100, (txn.allocated / txn.amount) * 100) : 0}%"></div></div>
        </div>

        ${links.length > 0 ? `
        <table style="margin-bottom:1rem">
            <thead><tr><th>Type</th><th>Document</th><th>Amount</th><th></th></tr></thead>
            <tbody>
                ${links.map(l => `<tr>
                    <td><span class="badge badge-${l.document_type}">${l.document_type}</span></td>
                    <td>#${l.document_id}</td>
                    <td class="money">${formatMoney(l.amount)}</td>
                    <td><button class="btn btn-danger btn-sm" onclick="unlinkTransaction(${txnId}, ${l.id})">Remove</button></td>
                </tr>`).join('')}
            </tbody>
        </table>` : '<p style="color:var(--text-muted);margin-bottom:1rem">No links yet</p>'}

        ${unallocated > 0 && suggestions.length > 0 ? `
        <h3 style="font-size:0.9rem;margin-bottom:0.5rem">Suggested Matches</h3>
        <table style="margin-bottom:1rem">
            <thead><tr><th>Type</th><th>Reference</th><th>Date</th><th>Unallocated</th><th>Confidence</th><th></th></tr></thead>
            <tbody>
                ${suggestions.map(s => {
                    const linkAmt = Math.min(unallocated, s.unallocated);
                    return `<tr>
                        <td><span class="badge badge-${s.document_type}">${s.document_type.replace('_', ' ')}</span></td>
                        <td>${esc(s.document_ref || '#' + s.document_id)}</td>
                        <td>${formatDate(s.document_date)}</td>
                        <td class="money">${formatMoney(s.unallocated)}</td>
                        <td>${confidenceBadgeHtml(s.confidence)}</td>
                        <td>${s.linkable ? `<button class="btn btn-primary btn-sm" onclick="quickLinkTransaction(${txnId}, '${s.document_type}', ${s.document_id}, ${linkAmt})">Link</button>` : ''}</td>
                    </tr>`;
                }).join('')}
            </tbody>
        </table>
        ` : ''}

        ${unallocated > 0 ? `
        <h3 style="font-size:0.9rem;margin-bottom:0.75rem">Add Link</h3>
        <form onsubmit="linkTransaction(event, ${txnId})">
            <div class="form-row">
                <div class="form-group">
                    <label>Type</label>
                    <select class="form-control" name="document_type" onchange="updateDocOptions(this.value)">
                        <option value="bill">Bill</option>
                        <option value="invoice">Invoice</option>
                        <option value="payout">Payout</option>
                    </select>
                </div>
                <div class="form-group">
                    <label>Document</label>
                    <select class="form-control" name="document_id" id="link-doc-select">
                        ${bills.filter(b => b.unallocated > 0).map(b => `<option value="${b.id}">#${b.id} ${b.bill_number || ''} (${formatMoney(b.unallocated)} free)</option>`).join('')}
                    </select>
                </div>
            </div>
            <div class="form-group">
                <label>Amount (₹) — max ${formatMoney(unallocated)}</label>
                <input class="form-control" name="amount" type="number" step="0.01" max="${toRupees(unallocated)}" required>
            </div>
            <div class="form-actions">
                <button type="submit" class="btn btn-primary btn-sm">Link</button>
            </div>
        </form>

        <script>
            window._linkBills = ${JSON.stringify(bills.filter(b => b.unallocated > 0))};
            window._linkInvoices = ${JSON.stringify(invoices.filter(i => i.unallocated > 0))};
            window._linkPayouts = ${JSON.stringify(payouts.map(p => ({ ...p, unallocated: p.final_payout_amt - (p.allocated || 0) })).filter(p => p.unallocated > 0))};
        </script>
        ` : ''}
    `);
}

async function quickLinkTransaction(txnId, docType, docId, amountPaise) {
    await postTransactionLink(txnId, docType, docId, amountPaise, () => showTransactionLinks(txnId));
}

function updateDocOptions(type) {
    const sel = document.getElementById('link-doc-select');
    let items = [];
    if (type === 'bill') items = window._linkBills;
    else if (type === 'invoice') items = window._linkInvoices;
    else if (type === 'payout') items = window._linkPayouts;

    sel.innerHTML = items.map(d => {
        let num = '';
        if (type === 'bill') num = d.bill_number;
        else if (type === 'invoice') num = d.invoice_number;
        else if (type === 'payout') num = `${d.platform} - ${d.outlet_name}`;
        return `<option value="${d.id}">#${d.id} ${num || ''} (${formatMoney(d.unallocated)} free)</option>`;
    }).join('');
}

async function linkTransaction(e, txnId) {
    e.preventDefault();
    const f = e.target;
    try {
        await api(`/transactions/${txnId}/links`, {
            method: 'POST',
            body: JSON.stringify({
                document_type: f.document_type.value,
                document_id: parseInt(f.document_id.value),
                amount: parseFloat(f.amount.value || 0),
            }),
        });
        showTransactionLinks(txnId);
    } catch (err) {
        alert('Error: ' + err.message);
    }
}

async function unlinkTransaction(txnId, linkId) {
    if (!confirm('Remove this link?')) return;
    await api(`/transactions/${txnId}/links/${linkId}`, { method: 'DELETE' });
    showTransactionLinks(txnId);
}

// ===== Recurring Payments =====
async function renderRecurringPayments(params) {
    const status = params?.get('status') || '';
    const type = params?.get('type') || '';
    let url = '/recurring-payments?';
    if (status) url += 'status=' + encodeURIComponent(status) + '&';
    if (type) url += 'type=' + encodeURIComponent(type);

    const [payments, accounts, contacts] = await Promise.all([
        api(url),
        api('/accounts'),
        api('/contacts'),
    ]);
    window._recurringAccounts = accounts;
    window._recurringContacts = contacts;

    el().innerHTML = `
        <div class="section-header">
            <h1>Recurring Payments</h1>
            <div style="display:flex;gap:0.5rem">
                <select id="rp-status-filter" class="form-control">
                    <option value="">— All Statuses —</option>
                    ${['active', 'paused', 'cancelled', 'completed'].map(s => `<option value="${s}" ${status === s ? 'selected' : ''}>${s.charAt(0).toUpperCase() + s.slice(1)}</option>`).join('')}
                </select>
                <select id="rp-type-filter" class="form-control">
                    <option value="">— All Types —</option>
                    <option value="income" ${type === 'income' ? 'selected' : ''}>Income</option>
                    <option value="expense" ${type === 'expense' ? 'selected' : ''}>Expense</option>
                </select>
                <button class="btn btn-primary" onclick="showRecurringPaymentForm()">+ New</button>
            </div>
        </div>
        <div class="table-wrap">
            <table>
                <thead><tr><th>Name</th><th>Type</th><th>Amount</th><th>Frequency</th><th>Account</th><th>Contact</th><th>Next Due</th><th>Status</th><th>Actions</th></tr></thead>
                <tbody>
                    ${payments.length === 0 ? '<tr><td colspan="9" class="empty-state">No recurring payments found</td></tr>' :
            payments.map(p => `<tr>
                        <td>
                            <strong>${esc(p.name)}</strong>
                            ${p.description ? `<br><small style="color:var(--text-muted)">${esc(p.description)}</small>` : ''}
                        </td>
                        <td><span class="badge badge-${esc(p.type)}">${esc(p.type)}</span></td>
                        <td class="money ${p.type === 'income' ? 'money-income' : 'money-expense'}">${formatMoney(p.amount)}</td>
                        <td>${p.interval > 1 ? 'Every ' + esc(p.interval) + ' ' + esc(p.frequency) : esc(p.frequency.charAt(0).toUpperCase() + p.frequency.slice(1))}</td>
                        <td>${esc(p.account_name || '—')}</td>
                        <td>${esc(p.contact_name || '—')}</td>
                        <td>${esc(formatDate(p.next_due_date))}</td>
                        <td><span class="badge badge-${esc(p.status)}">${esc(p.status)}</span></td>
                        <td class="actions-cell">
                            <button class="btn btn-ghost btn-sm" onclick="showRecurringPaymentSuggestions(${p.id})">Suggestions</button>
                            <button class="btn btn-ghost btn-sm" onclick="showRecurringPaymentForm(${p.id})">Edit</button>
                            <button class="btn btn-danger btn-sm" onclick="deleteRecurringPayment(${p.id})">Delete</button>
                        </td>
                    </tr>`).join('')}
                </tbody>
            </table>
        </div>
    `;

    document.getElementById('rp-status-filter').addEventListener('change', function () {
        navigate('recurring-payments', { status: this.value, type: document.getElementById('rp-type-filter').value });
    });
    document.getElementById('rp-type-filter').addEventListener('change', function () {
        navigate('recurring-payments', { status: document.getElementById('rp-status-filter').value, type: this.value });
    });
}

async function showRecurringPaymentForm(id) {
    let data = {
        name: '', type: 'expense', amount: 0, account_id: '', contact_id: null,
        frequency: 'monthly', interval: 1, start_date: '', end_date: null,
        next_due_date: null, status: 'active', description: null, reference: null,
    };
    if (id) data = await api(`/recurring-payments/${id}`);
    const accounts = window._recurringAccounts || await api('/accounts');
    const contacts = window._recurringContacts || await api('/contacts');

    openModal(id ? 'Edit Recurring Payment' : 'New Recurring Payment', `
        <form onsubmit="saveRecurringPayment(event, ${id || 'null'})">
            <div class="form-group">
                <label>Name</label>
                <input class="form-control" name="name" value="${esc(data.name)}" required>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>Type</label>
                    <select class="form-control" name="type">
                        <option value="expense" ${data.type === 'expense' ? 'selected' : ''}>Expense</option>
                        <option value="income" ${data.type === 'income' ? 'selected' : ''}>Income</option>
                    </select>
                </div>
                <div class="form-group">
                    <label>Amount (₹)</label>
                    <input class="form-control" name="amount" type="number" step="0.01" value="${esc(toRupees(data.amount))}" required>
                </div>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>Account</label>
                    <select class="form-control" name="account_id" required>
                        <option value="">Select account</option>
                        ${accounts.map(a => `<option value="${a.id}" ${data.account_id == a.id ? 'selected' : ''}>${esc(a.name)}</option>`).join('')}
                    </select>
                </div>
                <div class="form-group">
                    <label>Contact (optional)</label>
                    <select class="form-control" name="contact_id">
                        <option value="">— None —</option>
                        ${contacts.map(c => `<option value="${c.id}" ${data.contact_id == c.id ? 'selected' : ''}>${esc(c.name)}</option>`).join('')}
                    </select>
                </div>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>Frequency</label>
                    <select class="form-control" name="frequency">
                        ${['daily', 'weekly', 'monthly', 'quarterly', 'yearly'].map(f => `<option value="${f}" ${data.frequency === f ? 'selected' : ''}>${f.charAt(0).toUpperCase() + f.slice(1)}</option>`).join('')}
                    </select>
                </div>
                <div class="form-group">
                    <label>Interval (every N)</label>
                    <input class="form-control" name="interval" type="number" min="1" value="${esc(data.interval || 1)}" required>
                </div>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>Start Date</label>
                    <input class="form-control" name="start_date" type="date" value="${esc(data.start_date || '')}" required>
                </div>
                <div class="form-group">
                    <label>End Date (optional)</label>
                    <input class="form-control" name="end_date" type="date" value="${esc(data.end_date || '')}">
                </div>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label>Next Due Date (optional)</label>
                    <input class="form-control" name="next_due_date" type="date" value="${esc(data.next_due_date || '')}">
                </div>
                <div class="form-group">
                    <label>Status</label>
                    <select class="form-control" name="status">
                        ${['active', 'paused', 'cancelled', 'completed'].map(s => `<option value="${s}" ${data.status === s ? 'selected' : ''}>${s.charAt(0).toUpperCase() + s.slice(1)}</option>`).join('')}
                    </select>
                </div>
            </div>
            <div class="form-group">
                <label>Description (optional)</label>
                <input class="form-control" name="description" value="${esc(data.description || '')}">
            </div>
            <div class="form-group">
                <label>Reference (optional)</label>
                <input class="form-control" name="reference" value="${esc(data.reference || '')}">
            </div>
            <div class="form-actions">
                <button type="button" class="btn btn-ghost" onclick="closeModal()">Cancel</button>
                <button type="submit" class="btn btn-primary">${id ? 'Update' : 'Create'}</button>
            </div>
        </form>
    `);
}

async function saveRecurringPayment(e, id) {
    e.preventDefault();
    const f = e.target;
    const body = JSON.stringify({
        name: f.name.value,
        type: f.type.value,
        amount: parseFloat(f.amount.value || 0),
        account_id: parseInt(f.account_id.value),
        contact_id: f.contact_id.value ? parseInt(f.contact_id.value) : null,
        frequency: f.frequency.value,
        interval: parseInt(f.interval.value),
        start_date: f.start_date.value,
        end_date: f.end_date.value || null,
        next_due_date: f.next_due_date.value || null,
        status: f.status.value,
        description: f.description.value || null,
        reference: f.reference.value || null,
    });
    try {
        if (id) await api(`/recurring-payments/${id}`, { method: 'PUT', body });
        else await api('/recurring-payments', { method: 'POST', body });
        closeModal();
        renderRecurringPayments(getSection().params);
    } catch (err) {
        alert('Error: ' + err.message);
    }
}

async function deleteRecurringPayment(id) {
    if (!confirm('Delete this recurring payment?')) return;
    await api(`/recurring-payments/${id}`, { method: 'DELETE' });
    renderRecurringPayments(getSection().params);
}

async function showRecurringPaymentSuggestions(id) {
    const [rp, suggestions] = await Promise.all([
        api(`/recurring-payments/${id}`),
        api(`/recurring-payments/${id}/match-suggestions`),
    ]);
    openModal(`Suggested Transactions — ${String(rp.name)}`, `
        <p style="color:var(--text-muted);font-size:0.85rem;margin-bottom:1rem">
            Informational only · ${formatMoney(rp.amount)} · ${esc(rp.frequency)}
        </p>
        ${suggestions.length === 0 ? '<p style="color:var(--text-muted)">No matching transactions found.</p>' : `
        <table style="margin-bottom:1rem">
            <thead><tr><th>Date</th><th>Description</th><th>Amount</th><th>Confidence</th></tr></thead>
            <tbody>
                ${suggestions.map(s => `<tr>
                    <td>${formatDate(s.transaction_date)}</td>
                    <td>${esc(s.description || s.reference || '—')}</td>
                    <td class="money">${formatMoney(s.amount)}</td>
                    <td>${confidenceBadgeHtml(s.confidence)}</td>
                </tr>`).join('')}
            </tbody>
        </table>`}
        <div class="form-actions">
            <button class="btn btn-primary" onclick="closeModal()">Close</button>
        </div>
    `);
}

// ===== Auth UI =====
function showAuthOverlay() {
    document.getElementById('auth-overlay').classList.remove('hidden');
    document.getElementById('app').style.display = 'none';
    showAuthView('login');
}

function hideAuthOverlay() {
    document.getElementById('auth-overlay').classList.add('hidden');
    document.getElementById('app').style.display = '';
}

function showAuthView(view) {
    document.getElementById('auth-login-view').classList.toggle('hidden', view !== 'login');
    document.getElementById('auth-register-view').classList.toggle('hidden', view !== 'register');
    const authErr = document.getElementById('auth-error');
    authErr.classList.add('hidden');
    authErr.classList.remove('auth-success');
    document.getElementById('register-error').classList.add('hidden');
}

async function handleLogin(event) {
    event.preventDefault();
    const errEl = document.getElementById('auth-error');
    errEl.classList.add('hidden');
    errEl.classList.remove('auth-success');
    const btn = document.getElementById('login-btn');
    btn.disabled = true;
    btn.textContent = 'Signing in…';
    try {
        const res = await fetch('/api/auth/login', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                email: document.getElementById('login-email').value,
                password: document.getElementById('login-password').value,
            }),
        });
        const json = await res.json();
        if (!res.ok) {
            throw new Error(json.error || 'Login failed');
        }
        const token = json.data?.token || json.token;
        if (!token) throw new Error('No token received from server');
        setToken(token);
        hideAuthOverlay();
        const { section, params } = getSection();
        renderSection(section, params);
    } catch (err) {
        errEl.textContent = err.message;
        errEl.classList.remove('hidden');
    } finally {
        btn.disabled = false;
        btn.textContent = 'Sign In';
    }
}

async function handleRegister(event) {
    event.preventDefault();
    const errEl = document.getElementById('register-error');
    errEl.classList.add('hidden');
    const btn = document.getElementById('register-btn');
    btn.disabled = true;
    btn.textContent = 'Creating account…';
    try {
        const res = await fetch('/api/auth/register', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                org_name: document.getElementById('register-org').value,
                email: document.getElementById('register-email').value,
                password: document.getElementById('register-password').value,
            }),
        });
        const json = await res.json();
        if (!res.ok) {
            throw new Error(json.error || 'Registration failed');
        }
        // Switch to login after successful registration
        showAuthView('login');
        document.getElementById('login-email').value = document.getElementById('register-email').value;
        // Show a success message in the login view
        const loginMsg = document.getElementById('auth-error');
        loginMsg.textContent = 'Account created! Please sign in.';
        loginMsg.classList.remove('hidden');
        loginMsg.classList.add('auth-success');
    } catch (err) {
        errEl.textContent = err.message;
        errEl.classList.remove('hidden');
    } finally {
        btn.disabled = false;
        btn.textContent = 'Create Account';
    }
}

function logout() {
    clearToken();
    showAuthOverlay();
}

// ===== Init =====
if (!isAuthenticated()) {
    showAuthOverlay();
} else {
    const { section, params } = getSection();
    renderSection(section, params);
}
