package webhooks

import (
	"context"
	"time"

	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/manager"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	openshiftconfigv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubevirt/hyperconverged-cluster-operator/controllers/commontestutils"
	hcoutil "github.com/kubevirt/hyperconverged-cluster-operator/pkg/util"
)

var _ = Describe("HyperconvergedController", func() {

	Describe("Controller setup", func() {

		Context("Setup", func() {

			It("Should setup the controller if on Openshift", func() {
				resources := []client.Object{}
				cl := commontestutils.InitClient(resources)

				ci := commontestutils.ClusterInfoMock{}
				Expect(ci.IsOpenshift()).To(BeTrue())

				mgr, err := commontestutils.NewManagerMock(&rest.Config{}, manager.Options{}, cl, logger)
				Expect(err).ToNot(HaveOccurred())
				mockmgr, ok := mgr.(*commontestutils.ManagerMock)
				Expect(ok).To(BeTrue())

				// we should have no runnable before registering the controller
				Expect(mockmgr.GetRunnables()).To(BeEmpty())

				// we should have one runnable after registering it on Openshift
				Expect(RegisterReconciler(mgr, ci)).To(Succeed())
				Expect(mockmgr.GetRunnables()).To(HaveLen(1))
			})

			It("Should not setup the controller if not on Openshift", func() {
				resources := []client.Object{}
				cl := commontestutils.InitClient(resources)

				ci := hcoutil.GetClusterInfo()
				Expect(ci.IsOpenshift()).To(BeFalse())

				mgr, err := commontestutils.NewManagerMock(&rest.Config{}, manager.Options{}, cl, logger)
				Expect(err).ToNot(HaveOccurred())
				mockmgr, ok := mgr.(*commontestutils.ManagerMock)
				Expect(ok).To(BeTrue())

				// we should have no runnable before registering the controller
				Expect(mockmgr.GetRunnables()).To(BeEmpty())

				// we should have still no runnable after registering if not on Openshift
				Expect(RegisterReconciler(mgr, ci)).To(Succeed())
				Expect(mockmgr.GetRunnables()).To(BeEmpty())
			})

		})

	})

	Describe("Reconcile APIServer CR", func() {

		Context("APIServer CR", func() {

			It("Should refresh cached APIServer if the reconciliation is caused by a change there", func() {

				initialTLSSecurityProfile := &openshiftconfigv1.TLSSecurityProfile{
					Type:         openshiftconfigv1.TLSProfileIntermediateType,
					Intermediate: &openshiftconfigv1.IntermediateTLSProfile{},
				}
				customTLSSecurityProfile := &openshiftconfigv1.TLSSecurityProfile{
					Type:   openshiftconfigv1.TLSProfileModernType,
					Modern: &openshiftconfigv1.ModernTLSProfile{},
				}

				clusterVersion := &openshiftconfigv1.ClusterVersion{
					ObjectMeta: metav1.ObjectMeta{
						Name: "version",
					},
					Spec: openshiftconfigv1.ClusterVersionSpec{
						ClusterID: "clusterId",
					},
				}

				infrastructure := &openshiftconfigv1.Infrastructure{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
					Status: openshiftconfigv1.InfrastructureStatus{
						ControlPlaneTopology:   openshiftconfigv1.HighlyAvailableTopologyMode,
						InfrastructureTopology: openshiftconfigv1.HighlyAvailableTopologyMode,
						PlatformStatus: &openshiftconfigv1.PlatformStatus{
							Type: "mocked",
						},
					},
				}

				ingress := &openshiftconfigv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
					Spec: openshiftconfigv1.IngressSpec{
						Domain: "domain",
					},
				}

				apiServer := &openshiftconfigv1.APIServer{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
					Spec: openshiftconfigv1.APIServerSpec{
						TLSSecurityProfile: initialTLSSecurityProfile,
					},
				}

				dns := &openshiftconfigv1.DNS{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
					Spec: openshiftconfigv1.DNSSpec{
						BaseDomain: commontestutils.BaseDomain,
					},
				}

				ipv4network := &openshiftconfigv1.Network{
					ObjectMeta: metav1.ObjectMeta{
						Name: "cluster",
					},
					Status: openshiftconfigv1.NetworkStatus{
						ClusterNetwork: []openshiftconfigv1.ClusterNetworkEntry{
							{
								CIDR: "10.128.0.0/14",
							},
						},
					},
				}

				resources := []client.Object{clusterVersion, infrastructure, ingress, apiServer, dns, ipv4network}
				cl := commontestutils.InitClient(resources)

				Expect(hcoutil.GetClusterInfo().Init(context.TODO(), cl, logger)).To(Succeed())
				ci := hcoutil.GetClusterInfo()
				// We should have corrctly mocked all the Openshift resources needed by clusterInfo
				Expect(ci.IsOpenshift()).To(BeTrue())

				Expect(initialTLSSecurityProfile).ToNot(Equal(customTLSSecurityProfile), "customTLSSecurityProfile should be a different value")
				Expect(ci.GetTLSSecurityProfile(nil)).To(Equal(initialTLSSecurityProfile), "should return the initial value)")

				r := ReconcileAPIServer{
					client: cl,
					ci:     ci,
				}

				request := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name: "cluster",
					},
				}

				// Reconcile to get all related objects under HCO's status
				res, err := r.Reconcile(context.TODO(), request)
				Expect(err).ToNot(HaveOccurred())
				Expect(res.RequeueAfter).To(Equal(1 * time.Minute))

				// Update ApiServer CR
				apiServer.Spec.TLSSecurityProfile = customTLSSecurityProfile
				Expect(cl.Update(context.TODO(), apiServer)).To(Succeed())
				Expect(hcoutil.GetClusterInfo().GetTLSSecurityProfile(nil)).To(Equal(initialTLSSecurityProfile), "should still return the cached value (initial value)")

				// Reconcile again to refresh ApiServer CR in memory
				res, err = r.Reconcile(context.TODO(), request)
				Expect(err).ToNot(HaveOccurred())
				Expect(res.RequeueAfter).To(Equal(1 * time.Minute))

				Expect(hcoutil.GetClusterInfo().GetTLSSecurityProfile(nil)).To(Equal(customTLSSecurityProfile), "should return the up-to-date value")

			})

		})

	})

})
