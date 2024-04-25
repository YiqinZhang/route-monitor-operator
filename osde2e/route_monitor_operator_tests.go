// DO NOT REMOVE TAGS BELOW. IF ANY NEW TEST FILES ARE CREATED UNDER /osde2e, PLEASE ADD THESE TAGS TO THEM IN ORDER TO BE EXCLUDED FROM UNIT TESTS. //go:build osde2e
//go:build osde2e
// +build osde2e

package osde2etests

import (
	"context"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift/osde2e-common/pkg/clients/openshift"
	"github.com/openshift/osde2e-common/pkg/clients/prometheus"
	. "github.com/openshift/osde2e-common/pkg/gomega/assertions"
	. "github.com/openshift/osde2e-common/pkg/gomega/matchers"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var routeMonitorOperatorTestName string = "[Suite: informing] [OSD] Route Monitor Operator (rmo)"

var _ = Describe("route-monitor-operator", func() {
	var (
		clusterID         string
		k8s               *openshift.Client
		prom              *prometheus.Client
		operatorNamespace = "openshift-route-monitor-operator"
		serviceName       = "Route Monitor Operator"
		deploymentName    = "route-monitor-operator-controller-manager"
		rolePrefix        = "route-monitor-operator"
		clusterRolePrefix = "route-monitor-operator"
		operatorName      = "route-monitor-operator"
	)
	const (
		defaultDesiredReplicas int32 = 1
	)
	BeforeAll(func(ctx context.Context) {
		log.SetLogger(GinkgoLogr)

		clusterID = os.Getenv("OCM_CLUSTER_ID")
		Expect(clusterID).ShouldNot(BeEmpty(), "failed to find OCM_CLUSTER_ID environment variable")

		var err error
		k8s, err = openshift.New(GinkgoLogr)
		Expect(err).ShouldNot(HaveOccurred(), "unable to setup k8s client")

		prom, err = prometheus.New(ctx, k8s)

		Expect(err).ShouldNot(HaveOccurred(), "unable to setup prometheus client")

	})

	It("is installed", func(ctx context.Context) {
		By("checking the namespace exists")
		err := k8s.Get(ctx, operatorNamespace, "", &corev1.Namespace{})
		Expect(err).ShouldNot(HaveOccurred(), "namespace %s not found", operatorNamespace)

		By("checking the role exists")
		var roles rbacv1.RoleList
		err = k8s.WithNamespace(operatorNamespace).List(ctx, &roles)
		Expect(err).ShouldNot(HaveOccurred(), "failed to list roles")
		Expect(&roles).Should(ContainItemWithPrefix(rolePrefix), "unable to find roles with prefix %s", rolePrefix)

		By("checking the rolebinding exists")
		var rolebindings rbacv1.RoleBindingList
		err = k8s.List(ctx, &rolebindings)
		Expect(err).ShouldNot(HaveOccurred(), "failed to list rolebindings")
		Expect(&rolebindings).Should(ContainItemWithPrefix(rolePrefix), "unable to find rolebindings with prefix %s", rolePrefix)

		By("checking the clusterrole exists")
		var clusterRoles rbacv1.ClusterRoleList
		err = k8s.List(ctx, &clusterRoles)
		Expect(err).ShouldNot(HaveOccurred(), "failed to list clusterroles")
		Expect(&clusterRoles).Should(ContainItemWithPrefix(clusterRolePrefix), "unable to find cluster role with prefix %s", clusterRolePrefix)

		By("checking the clusterrolebinding exists")
		var clusterRoleBindings rbacv1.ClusterRoleBindingList
		err = k8s.List(ctx, &clusterRoleBindings)
		Expect(err).ShouldNot(HaveOccurred(), "unable to list clusterrolebindings")
		Expect(&clusterRoleBindings).Should(ContainItemWithPrefix(clusterRolePrefix), "unable to find clusterrolebinding with prefix %s", clusterRolePrefix)

		By("checking the service exists")
		err = k8s.Get(ctx, serviceName, operatorNamespace, &corev1.Service{})
		Expect(err).ShouldNot(HaveOccurred(), "service %s/%s not found", operatorNamespace, serviceName)

		By("checking the deployment exists and is available")
		EventuallyDeployment(ctx, k8s, deploymentName, operatorNamespace).Should(BeAvailable())
	})

	It("can be upgraded", func(ctx context.Context) {
		By("forcing operator upgrade")
		err := k8s.UpgradeOperator(ctx, operatorName, operatorNamespace)
		Expect(err).NotTo(HaveOccurred(), "operator upgrade failed")
	})

	It("rmo Route Monitor Operator regression for console", func(ctx context.Context) {
		const (
			consoleNamespace = "openshift-route-monitor-operator"
			consoleName      = "console"
		)
		results, err := prom.InstantQuery(ctx, `up{job="route-monitor-operator"}`)
		Expect(err).ShouldNot(HaveOccurred(), "failed to query prometheus")

		result := results[0].Value
		Expect(int(result)).Should(BeNumerically("==", 1), "prometheus exporter is not healthy")
		// Check for ServiceMonitor existence
		_, err = prom.InstantQuery()
		//ServiceMonitors(consoleNamespace).Get(ctx, consoleName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred(), "Could not get console serviceMonitor")
		// Check for PrometheusRule existence
		_, err = prom.MonitoringV1().PrometheusRules(consoleNamespace).Get(ctx, consoleName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred(), "Could not get console prometheusRule")
	})
})

func testRouteMonitorCreationWorks(k8s *openshift.Client) {
	Context("rmo Route Monitor Operator integration test", func() {
		pollingDuration := 10 * time.Minute
		It("Creates and deletes a RouteMonitor to see if it works accordingly", func(ctx context.Context) {
			routeMonitorNamespace := "route-monitor-operator"
			const routeMonitorName = "routemonitor-e2e-test"

			By("Creating a pod, service, and route to monitor with a ServiceMonitor and PrometheusRule")
			// Create Pod
			pod := createSamplePod(routeMonitorName, routeMonitorNamespace)
			err := k8s.Create(ctx, &pod)
			Expect(err).NotTo(HaveOccurred(), "Couldn't create a testing pod")

			// Wait for Pod to be running
			err = waitForPodRunning(ctx, routeMonitorNamespace, routeMonitorName)
			Expect(err).NotTo(HaveOccurred(), "Pod is not running")

			// Create Service
			svc := createSampleService(routeMonitorName, routeMonitorNamespace)
			err = k8s.Create(ctx, &svc)
			Expect(err).NotTo(HaveOccurred(), "Couldn't create a testing service")

			// Create Route
			appRoute := createSampleRoute(routeMonitorName, routeMonitorNamespace)
			err = k8s.Create(ctx, &appRoute)
			Expect(err).NotTo(HaveOccurred(), "Couldn't create application route")

			Eventually(func() bool {
				_, err := k8s.CoreV1().Services(routeMonitorNamespace).Get(ctx, routeMonitorName, metav1.GetOptions{})
				if err != nil {
					return false
				}
				return true
			}, pollingDuration, time.Second).Should(BeTrue(), "Failed to verify that resources were created")

			By("Deleting the sample RouteMonitor")
			err := k8s.CoreV1().Services(routeMonitorNamespace).Delete(ctx, routeMonitorName, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred(), "Couldn't delete the service")

			Eventually(func() bool {
				_, err := k8s.CoreV1().Services(routeMonitorNamespace).Get(ctx, routeMonitorName, metav1.GetOptions{})
				return err != nil // Expect an error since the resource should not exist
			}, pollingDuration, time.Second).Should(BeTrue(), "Service still exists after deletion")
		})
	})
}
